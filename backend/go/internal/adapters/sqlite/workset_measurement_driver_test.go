//nolint:testpackage // The opt-in harness wraps the repository's private driver and scan pools.
package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type worksetSQLMeasurement struct {
	mu    sync.Mutex
	paths map[string]worksetSQLSample
}

type worksetSQLSample struct {
	count, failures int
	begin, held     time.Duration
}

func (m *worksetSQLMeasurement) record(path string, begin, held time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.paths[path]
	s.count++
	if err != nil {
		s.failures++
	}
	s.begin = max(s.begin, begin)
	s.held = max(s.held, held)
	m.paths[path] = s
}

func (m *worksetSQLMeasurement) report(t *testing.T) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.paths))
	for path := range m.paths {
		keys = append(keys, path)
	}
	slices.Sort(keys)
	t.Log("sql_path samples failures max_begin_ms max_held_ms max_autocommit_ms")
	for _, path := range keys {
		s := m.paths[path]
		switch {
		case strings.Contains(path, "/phase/"):
			t.Logf("sql_phase %s %d %d elapsed_ms=%.3f",
				path, s.count, s.failures, float64(s.held)/float64(time.Millisecond))
		case strings.HasSuffix(path, "/autocommit"):
			t.Logf("sql %s %d %d n/a n/a %.3f", path, s.count, s.failures, float64(s.held)/float64(time.Millisecond))
		default:
			t.Logf("sql %s %d %d %.3f %.3f n/a", path, s.count, s.failures,
				float64(s.begin)/float64(time.Millisecond), float64(s.held)/float64(time.Millisecond))
		}
	}
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
			return strings.Split(frame.Function, "/adapters/sqlite.")[1]
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
	started := time.Now()
	beginner, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, fmt.Errorf("measurement driver requires ConnBeginTx")
	}
	tx, err := beginner.BeginTx(ctx, options)
	wait := time.Since(started)
	if err != nil {
		c.measurement.record(path, wait, 0, err)
		return nil, err
	}
	measured := &worksetMeasuredTx{
		Tx:       tx,
		conn:     c,
		path:     path,
		begin:    wait,
		started:  time.Now(),
		readOnly: options.ReadOnly,
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
	elapsed := time.Since(started)
	if c.transaction != nil && c.transaction.path == "(*Repository).deleteLibrary" &&
		command != "COMMIT" && command != "ROLLBACK" {
		phase := "record_retirement"
		switch {
		case strings.HasPrefix(command, "DELETE FROM WORK_COPIES"):
			phase = "copy_unbinding"
		case strings.HasPrefix(command, "DELETE FROM INVENTORY_DIRS"):
			phase = "inventory_deletion"
		case strings.HasPrefix(command, "DELETE FROM LIBRARIES"):
			phase = "library_deletion"
		}
		if c.transaction.phases == nil {
			c.transaction.phases = map[string]time.Duration{}
		}
		c.transaction.phases[phase] += elapsed
	}
	switch {
	case strings.HasPrefix(command, "BEGIN"):
		if strings.Contains(command, "DEFERRED") {
			path += "/temp"
		}
		if err != nil {
			c.measurement.record(path, elapsed, 0, err)
		} else {
			c.transaction = &worksetMeasuredTx{conn: c, path: path, begin: elapsed, started: time.Now()}
		}
	case command == "COMMIT" || command == "ROLLBACK":
		if c.transaction != nil {
			c.transaction.finish(err)
		}
	case c.transaction == nil && (strings.HasPrefix(command, "INSERT") || strings.HasPrefix(command, "UPDATE") || strings.HasPrefix(command, "DELETE")):
		// Autocommit combines lock acquisition and writing: it has no separately
		// observable BEGIN. Report its whole duration under its own path suffix.
		c.measurement.record(path+"/autocommit", 0, elapsed, err)
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
	command := strings.ToUpper(strings.TrimSpace(query))
	if c.transaction == nil &&
		(strings.HasPrefix(command, "UPDATE") || strings.HasPrefix(command, "INSERT") || strings.HasPrefix(command, "DELETE")) {
		path := worksetSQLPath() + "/autocommit"
		if err != nil {
			c.measurement.record(path, 0, time.Since(started), err)
		} else {
			return &worksetMeasuredRows{Rows: rows, measurement: c.measurement, path: path, started: started}, nil
		}
	}
	return rows, err
}

type worksetMeasuredTx struct {
	driver.Tx

	conn     *worksetMeasuredConn
	path     string
	begin    time.Duration
	started  time.Time
	readOnly bool
	phases   map[string]time.Duration
}

func (tx *worksetMeasuredTx) finish(err error) {
	if !tx.readOnly {
		tx.conn.measurement.record(tx.path, tx.begin, time.Since(tx.started), err)
		for phase, elapsed := range tx.phases {
			tx.conn.measurement.record(tx.path+"/phase/"+phase, 0, elapsed, err)
		}
	}
	tx.conn.transaction = nil
}
func (tx *worksetMeasuredTx) Commit() error   { err := tx.Tx.Commit(); tx.finish(err); return err }
func (tx *worksetMeasuredTx) Rollback() error { err := tx.Tx.Rollback(); tx.finish(err); return err }

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
	r.measurement.record(r.path, 0, time.Since(r.started), errors.Join(r.failure, err))
	return err
}
