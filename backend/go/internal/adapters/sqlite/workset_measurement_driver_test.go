//nolint:testpackage // The opt-in harness wraps the repository's private driver and scan pools.
package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type worksetSQLMeasurement struct {
	mu         sync.Mutex
	paths      map[string]worksetSQLSample
	stmts      map[worksetStmtKey]worksetStmtStat
	holds      []worksetInterval // main-db transaction holds, after BEGIN returned
	waits      []worksetInterval // BEGIN waits
	statements []worksetInterval // autocommit statement windows: lock wait and write are not separable
	merges     []worksetMerge
	imports    []worksetImportWait // per ImportMetadata call: Go queue + BEGIN
	readActive atomic.Bool
	readNanos  atomic.Int64
	// saveDelay slows each execution result save so a sampled session stays
	// running while the browser measures it (zero outside that window).
	saveDelay atomic.Int64
}

type worksetSQLSample struct {
	count, failures int
	begin, held     time.Duration
	totalHeld       time.Duration
}

// worksetStmtKey names a statement kind within a transaction path. COMMIT is
// the commit itself.
type worksetStmtKey struct{ path, kind string }

type worksetStmtStat struct {
	count      int
	max, total time.Duration
}

// worksetInterval is a held or waited span; the path is the waiting or holding one.
type worksetInterval struct {
	path       string
	start, end time.Time
}

// worksetImportWait is one ImportMetadata call's wait from starting to acquire
// the Go lock until db.Begin returned; the BEGIN part is [end-begin, end].
type worksetImportWait struct {
	end          time.Time
	queue, begin time.Duration
}

func (w worksetImportWait) total() time.Duration { return w.queue + w.begin }

func (w worksetImportWait) beginWindow() worksetInterval {
	return worksetInterval{path: "ImportMetadata/begin", start: w.end.Add(-w.begin), end: w.end}
}

func (m *worksetSQLMeasurement) recordImportWait(queue, begin time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.imports = append(m.imports, worksetImportWait{end: time.Now(), queue: queue, begin: begin})
}

// worksetMerge is one MergeSelectedStaging invocation.
type worksetMerge struct {
	wall, held time.Duration
	batches    int
}

func (a worksetInterval) overlap(b worksetInterval) time.Duration {
	latest := a.start
	if b.start.After(latest) {
		latest = b.start
	}
	earliest := a.end
	if b.end.Before(earliest) {
		earliest = b.end
	}
	return max(0, earliest.Sub(latest))
}

func (m *worksetSQLMeasurement) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.paths = map[string]worksetSQLSample{}
	m.stmts = nil
	m.holds, m.waits, m.statements, m.merges = nil, nil, nil, nil
}

// sample accumulates one path's totals; the caller holds m.mu.
func (m *worksetSQLMeasurement) sample(path string, begin, held time.Duration, err error) {
	s := m.paths[path]
	s.count++
	if err != nil {
		s.failures++
	}
	s.begin = max(s.begin, begin)
	s.held = max(s.held, held)
	s.totalHeld += held
	m.paths[path] = s
}

// recordTransaction takes the boundary timestamps observed at the operation
// sites: beginStart before BEGIN, holdStart after BEGIN returned, end after
// COMMIT/ROLLBACK returned. TEMP-only transactions never hold the main writer.
func (m *worksetSQLMeasurement) recordTransaction(path string, beginStart, holdStart, end time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sample(path, holdStart.Sub(beginStart), end.Sub(holdStart), err)
	if strings.HasSuffix(path, "/temp") {
		return
	}
	m.waits = append(m.waits, worksetInterval{path, beginStart, holdStart})
	if end.After(holdStart) {
		m.holds = append(m.holds, worksetInterval{path, holdStart, end})
	}
}

// recordStatement records an autocommit write. Its window includes lock
// acquisition, so it is kept apart from definite holds.
func (m *worksetSQLMeasurement) recordStatement(path string, start, end time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sample(path, 0, end.Sub(start), err)
	m.statements = append(m.statements, worksetInterval{path, start, end})
}

func (m *worksetSQLMeasurement) recordPhase(path string, elapsed time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sample(path, 0, elapsed, err)
}

func (m *worksetSQLMeasurement) recordStatements(path string, stats map[string]*worksetStmtStat) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stmts == nil {
		m.stmts = map[worksetStmtKey]worksetStmtStat{}
	}
	for kind, stat := range stats {
		total := m.stmts[worksetStmtKey{path, kind}]
		total.count += stat.count
		total.max = max(total.max, stat.max)
		total.total += stat.total
		m.stmts[worksetStmtKey{path, kind}] = total
	}
}

// recordMerge reports one MergeSelectedStaging call and the batch transactions
// that ran inside its window.
func (m *worksetSQLMeasurement) recordMerge(start, end time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	merge := worksetMerge{wall: end.Sub(start)}
	for _, hold := range m.holds {
		if hold.path == "mergeScopeBatch" && !hold.start.Before(start) && !hold.end.After(end) {
			merge.batches++
			merge.held += hold.end.Sub(hold.start)
		}
	}
	m.merges = append(m.merges, merge)
}

func milliseconds(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func (m *worksetSQLMeasurement) report(t *testing.T) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.paths))
	for path := range m.paths {
		keys = append(keys, path)
	}
	slices.Sort(keys)
	t.Log("sql_path samples failures max_begin_ms max_held_ms max_autocommit_ms total_held_ms")
	for _, path := range keys {
		s := m.paths[path]
		switch {
		case strings.Contains(path, "/phase/"):
			t.Logf("sql_phase %s %d %d elapsed_ms=%.3f", path, s.count, s.failures, milliseconds(s.held))
		case strings.HasSuffix(path, "/autocommit"):
			t.Logf("sql %s %d %d n/a n/a %.3f", path, s.count, s.failures, milliseconds(s.held))
		default:
			t.Logf("sql %s %d %d %.3f %.3f n/a %.3f", path, s.count, s.failures,
				milliseconds(s.begin), milliseconds(s.held), milliseconds(s.totalHeld))
		}
	}
	stmtKeys := slices.SortedFunc(maps.Keys(m.stmts), func(a, b worksetStmtKey) int {
		return strings.Compare(a.path+"\x00"+a.kind, b.path+"\x00"+b.kind)
	})
	for _, key := range stmtKeys {
		stat := m.stmts[key]
		if key.kind == worksetCommitKind || key.kind == worksetRollbackKind {
			t.Logf("sql_%s %s count=%d max_ms=%.3f total_ms=%.3f", strings.ToLower(key.kind),
				key.path, stat.count, milliseconds(stat.max), milliseconds(stat.total))
		} else {
			t.Logf("sql_stmt %s %q count=%d max_ms=%.3f total_ms=%.3f",
				key.path, key.kind, stat.count, milliseconds(stat.max), milliseconds(stat.total))
		}
	}
	byPath := map[string][]time.Duration{}
	for _, wait := range m.waits {
		byPath[wait.path] = append(byPath[wait.path], wait.end.Sub(wait.start))
	}
	for _, path := range slices.Sorted(maps.Keys(byPath)) {
		d := byPath[path]
		slices.Sort(d)
		over, _ := slices.BinarySearch(d, time.Second+1)
		over = len(d) - over
		t.Logf("sql_begin_p99 %s count=%d p99_ms=%.3f over_1s=%d", path, len(d),
			milliseconds(d[(len(d)*99+99)/100-1]), over)
	}
	m.reportImportWaits(t)
	for i, merge := range m.merges {
		t.Logf("sql_merge #%d wall_ms=%.3f batches=%d held_ms=%.3f",
			i+1, milliseconds(merge.wall), merge.batches, milliseconds(merge.held))
	}
	for _, wait := range m.waits {
		if wait.end.Sub(wait.start) <= time.Second {
			continue
		}
		t.Logf("sql_wait %s waited_ms=%.3f", wait.path, milliseconds(wait.end.Sub(wait.start)))
		for _, windows := range []struct {
			label     string
			intervals []worksetInterval
		}{{"sql_wait_hold", m.holds}, {"sql_wait_stmt", m.statements}} {
			overlap := map[string]time.Duration{}
			for _, interval := range windows.intervals {
				overlap[interval.path] += wait.overlap(interval)
			}
			for _, path := range slices.Sorted(maps.Keys(overlap)) {
				if overlap[path] > 0 {
					t.Logf("%s %s overlap_ms=%.3f", windows.label, path, milliseconds(overlap[path]))
				}
			}
		}
	}
}

func worksetPercentile99(d []time.Duration) time.Duration {
	sorted := slices.Sorted(slices.Values(d))
	return sorted[(len(sorted)*99+99)/100-1]
}

// reportImportWaits reports the combined ImportMetadata wait (Go queue plus
// BEGIN, computed per call), then queue and BEGIN separately for diagnosis. For
// the longest totals it splits queue from BEGIN and lists the other write paths
// holding the database during the BEGIN part.
func (m *worksetSQLMeasurement) reportImportWaits(t *testing.T) {
	t.Helper()
	if len(m.imports) == 0 {
		return
	}
	var totals, queues, begins []time.Duration
	over := 0
	for _, w := range m.imports {
		totals = append(totals, w.total())
		queues = append(queues, w.queue)
		begins = append(begins, w.begin)
		if w.total() > time.Second {
			over++
		}
	}
	t.Logf("import_wait_total count=%d max_ms=%.3f p99_ms=%.3f over_1s=%d", len(totals),
		milliseconds(slices.Max(totals)), milliseconds(worksetPercentile99(totals)), over)
	t.Logf("import_wait_queue max_ms=%.3f p99_ms=%.3f", milliseconds(slices.Max(queues)),
		milliseconds(worksetPercentile99(queues)))
	t.Logf("import_wait_begin max_ms=%.3f p99_ms=%.3f", milliseconds(slices.Max(begins)),
		milliseconds(worksetPercentile99(begins)))
	longest := slices.SortedFunc(slices.Values(m.imports), func(a, b worksetImportWait) int {
		return int(b.total() - a.total())
	})
	n := over
	if n == 0 {
		n = min(3, len(longest))
	}
	for _, w := range longest[:n] {
		t.Logf("import_wait_long total_ms=%.3f queue_ms=%.3f begin_ms=%.3f", milliseconds(w.total()),
			milliseconds(w.queue), milliseconds(w.begin))
		window := w.beginWindow()
		overlap := map[string]time.Duration{}
		for _, hold := range m.holds {
			overlap[hold.path] += window.overlap(hold)
		}
		for _, path := range slices.Sorted(maps.Keys(overlap)) {
			if overlap[path] > 0 {
				t.Logf("import_wait_long_begin_hold %s overlap_ms=%.3f", path, milliseconds(overlap[path]))
			}
		}
	}
}

// worksetCommitKind and worksetRollbackKind key how a transaction ended among
// its statements, so early-return rollbacks stay out of the commit timings.
const (
	worksetCommitKind   = "COMMIT"
	worksetRollbackKind = "ROLLBACK"
)

// worksetStatementKind is the first 30 characters of the whitespace-normalized SQL.
func worksetStatementKind(query string) string {
	kind := strings.Join(strings.Fields(query), " ")
	return kind[:min(len(kind), 30)]
}

// Driver timings exclude database/sql pool waiting. BEGIN includes SQLite's lock
// wait; held starts after successful BEGIN and includes COMMIT/ROLLBACK. TEMP-only
// transactions are identified separately because they never hold the main writer.
type worksetMeasuredDriver struct {
	driver.Driver

	measurement *worksetSQLMeasurement
}

func (d worksetMeasuredDriver) Open(dsn string) (driver.Conn, error) {
	c, err := d.Driver.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &worksetMeasuredConn{Conn: c, measurement: d.measurement}, nil
}

type worksetMeasuredConn struct {
	driver.Conn

	measurement *worksetSQLMeasurement
	transaction *worksetMeasuredTx
}

func worksetSQLPath() string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, "/adapters/sqlite.") &&
			!strings.Contains(frame.File, "workset_measurement") &&
			!strings.HasSuffix(frame.File, "scan_connection.go") {
			path := strings.Split(frame.Function, "/adapters/sqlite.")[1]
			// Keep the path name stable across the locking helper split.
			if path == "(*Repository).importMetadataLocked" {
				return "(*Repository).ImportMetadata"
			}
			return path
		}
		if !more {
			return "fixture"
		}
	}
}

func (c *worksetMeasuredConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *worksetMeasuredConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	path := worksetSQLPath()
	if delay := c.measurement.saveDelay.Load(); delay > 0 && path == "(*Repository).SaveExecutionComponentResult" {
		time.Sleep(time.Duration(delay))
	}
	beginStart := time.Now()
	beginner, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, fmt.Errorf("measurement driver requires ConnBeginTx")
	}
	tx, err := beginner.BeginTx(ctx, options)
	holdStart := time.Now()
	if err != nil {
		c.measurement.recordTransaction(path, beginStart, holdStart, holdStart, err)
		return nil, err
	}
	measured := &worksetMeasuredTx{
		Tx:         tx,
		conn:       c,
		path:       path,
		beginStart: beginStart,
		holdStart:  holdStart,
		readOnly:   options.ReadOnly,
	}
	c.transaction = measured
	return measured, nil
}

func (c *worksetMeasuredConn) ExecContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Result, error) {
	command := strings.ToUpper(strings.TrimSpace(query))
	var path string
	if c.transaction == nil {
		path = worksetSQLPath()
	}
	started := time.Now()
	executor, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, fmt.Errorf("measurement driver requires ExecerContext")
	}
	result, err := executor.ExecContext(ctx, query, args)
	finished := time.Now()
	elapsed := finished.Sub(started)
	if c.transaction != nil && command != "COMMIT" && command != "ROLLBACK" {
		c.transaction.statement(query, command, elapsed)
	}
	switch {
	case strings.HasPrefix(command, "BEGIN"):
		if strings.Contains(command, "DEFERRED") {
			path += "/temp"
		}
		if err != nil {
			c.measurement.recordTransaction(path, started, finished, finished, err)
		} else {
			c.transaction = &worksetMeasuredTx{conn: c, path: path, beginStart: started, holdStart: finished}
		}
	case command == "COMMIT" || command == "ROLLBACK":
		if c.transaction != nil {
			c.transaction.finish(command, started, finished, err)
		}
	case c.transaction == nil && (strings.HasPrefix(command, "INSERT") || strings.HasPrefix(command, "UPDATE") || strings.HasPrefix(command, "DELETE")):
		// Autocommit combines lock acquisition and writing: it has no separately
		// observable BEGIN, so its window is not a definite hold.
		c.measurement.recordStatement(path+"/autocommit", started, finished, err)
	}
	return result, err
}

func (c *worksetMeasuredConn) QueryContext(
	ctx context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	querier, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, fmt.Errorf("measurement driver requires QueryerContext")
	}
	started := time.Now()
	rows, err := querier.QueryContext(ctx, query, args)
	finished := time.Now()
	if c.measurement.readActive.Load() {
		c.measurement.readNanos.Add(int64(finished.Sub(started)))
		if rows != nil {
			return &worksetReadTimedRows{Rows: rows, elapsed: &c.measurement.readNanos}, err
		}
	}
	command := strings.ToUpper(strings.TrimSpace(query))
	if c.transaction != nil {
		if err != nil {
			c.transaction.statement(query, command, finished.Sub(started))
			return rows, err
		}
		// Fetching rows is part of the statement's time inside the transaction.
		return &worksetTxRows{Rows: rows, tx: c.transaction, query: query, command: command, started: started}, nil
	} else if strings.HasPrefix(command, "UPDATE") || strings.HasPrefix(command, "INSERT") || strings.HasPrefix(command, "DELETE") {
		path := worksetSQLPath() + "/autocommit"
		if err != nil {
			c.measurement.recordStatement(path, started, finished, err)
		} else {
			return &worksetMeasuredRows{Rows: rows, measurement: c.measurement, path: path, started: started}, nil
		}
	}
	return rows, err
}

type worksetMeasuredTx struct {
	driver.Tx

	conn       *worksetMeasuredConn
	path       string
	beginStart time.Time
	holdStart  time.Time
	readOnly   bool
	phases     map[string]time.Duration
	stmts      map[string]*worksetStmtStat
}

// statement times one statement inside the transaction, by kind.
func (tx *worksetMeasuredTx) statement(query, command string, elapsed time.Duration) {
	if tx.path == "(*Repository).deleteLibrary" {
		phase := "record_retirement"
		switch {
		case strings.HasPrefix(command, "DELETE FROM WORK_COPIES"):
			phase = "copy_unbinding"
		case strings.HasPrefix(command, "DELETE FROM INVENTORY_DIRS"):
			phase = "inventory_deletion"
		case strings.HasPrefix(command, "DELETE FROM LIBRARIES"):
			phase = "library_deletion"
		}
		if tx.phases == nil {
			tx.phases = map[string]time.Duration{}
		}
		tx.phases[phase] += elapsed
	}
	kind := worksetStatementKind(query)
	if tx.stmts == nil {
		tx.stmts = map[string]*worksetStmtStat{}
	}
	stat := tx.stmts[kind]
	if stat == nil {
		stat = &worksetStmtStat{}
		tx.stmts[kind] = stat
	}
	stat.count++
	stat.max = max(stat.max, elapsed)
	stat.total += elapsed
}

func (tx *worksetMeasuredTx) finish(kind string, endStart, end time.Time, err error) {
	if !tx.readOnly {
		m := tx.conn.measurement
		m.recordTransaction(tx.path, tx.beginStart, tx.holdStart, end, err)
		for phase, elapsed := range tx.phases {
			m.recordPhase(tx.path+"/phase/"+phase, elapsed, err)
		}
		if tx.stmts == nil {
			tx.stmts = map[string]*worksetStmtStat{}
		}
		tx.stmts[kind] = &worksetStmtStat{count: 1, max: end.Sub(endStart), total: end.Sub(endStart)}
		m.recordStatements(tx.path, tx.stmts)
	}
	tx.conn.transaction = nil
}

func (tx *worksetMeasuredTx) Commit() error {
	started := time.Now()
	err := tx.Tx.Commit()
	tx.finish(worksetCommitKind, started, time.Now(), err)
	return err
}

func (tx *worksetMeasuredTx) Rollback() error {
	started := time.Now()
	err := tx.Tx.Rollback()
	tx.finish(worksetRollbackKind, started, time.Now(), err)
	return err
}

// RETURNING keeps an autocommit write open until its rows are closed.
type worksetMeasuredRows struct {
	driver.Rows

	measurement *worksetSQLMeasurement
	path        string
	started     time.Time
	failure     error
}

func (r *worksetMeasuredRows) Next(values []driver.Value) error {
	err := r.Rows.Next(values)
	if err != nil && !errors.Is(err, io.EOF) {
		r.failure = err
	}
	return err
}
func (r *worksetMeasuredRows) Close() error {
	err := r.Rows.Close()
	r.measurement.recordStatement(r.path, r.started, time.Now(), errors.Join(r.failure, err))
	return err
}

// worksetTxRows times a query inside a transaction until its rows are closed.
type worksetTxRows struct {
	driver.Rows

	tx             *worksetMeasuredTx
	query, command string
	started        time.Time
}

func (r *worksetTxRows) Close() error {
	err := r.Rows.Close()
	r.tx.statement(r.query, r.command, time.Since(r.started))
	return err
}
