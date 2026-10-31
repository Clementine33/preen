package workset

import (
	"context"
	"time"
)

// attentionCap bounds the exception list of a members view.
const attentionCap = 50

// Member statuses of a session. memberStatus is their only definition.
const (
	MemberSucceeded = "succeeded"
	MemberFailed    = "failed"
	MemberCanceled  = "canceled"
	MemberRunning   = "running"
	MemberPending   = "pending"
)

// MemberSummary is one member's Member Result: its frozen components folded
// with their observed outcomes.
type MemberSummary struct {
	MemberID            string `json:"member_id"`
	RootPath            string `json:"root_path"`
	Status              string `json:"status"`
	TotalComponents     int    `json:"total_components"`
	CompletedComponents int    `json:"completed_components"`
	TotalOperations     int    `json:"total_operations"`
	CompletedOperations int    `json:"completed_operations"`
	ErrorMessage        string `json:"error_message"`
	InventorySyncError  string `json:"inventory_sync_error"`
}

// MemberCounts is the number of members in each status.
type MemberCounts struct {
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Canceled  int `json:"canceled"`
	Running   int `json:"running"`
	Pending   int `json:"pending"`
}

// ExecutionMembers is the `members` block of a view=members read: the counts,
// and the exception members (failed, canceled, running, or with an inventory
// sync warning) in frozen order, at most attentionCap of them.
type ExecutionMembers struct {
	Counts         MemberCounts    `json:"counts"`
	Attention      []MemberSummary `json:"attention"`
	AttentionTotal int             `json:"attention_total"`
}

// MemberEvent is the payload of a `member` event: the member's latest summary
// with the session's current counts.
type MemberEvent struct {
	Member         MemberSummary `json:"member"`
	Counts         MemberCounts  `json:"counts"`
	AttentionTotal int           `json:"attention_total"`
}

// memberStatus is the one rule that turns a member's components into a status.
// Running wins while the session is live and the member holds the current
// component; otherwise all succeeded, then any failed, then any canceled.
func memberStatus(running bool, total, succeeded, failed, canceled int) string {
	switch {
	case running:
		return MemberRunning
	case total > 0 && succeeded == total:
		return MemberSucceeded
	case failed > 0:
		return MemberFailed
	case canceled > 0:
		return MemberCanceled
	}
	return MemberPending
}

func isAttention(s MemberSummary) bool {
	switch s.Status {
	case MemberFailed, MemberCanceled, MemberRunning:
		return true
	}
	return s.InventorySyncError != ""
}

type memberRec struct {
	id, root string
	idx      []int // component indexes, frozen order
	ops      int
}

// memberFold holds a session's member summaries and keeps them current as
// results land and the current component moves, without rereading anything: it
// is built once from the frozen worklist, the members and the result digests.
type memberFold struct {
	recs     []memberRec
	owner    map[int]int    // component index -> member position
	byComp   map[string]int // component id -> member position
	byMember map[string]int // member id -> position
	digests  map[int]ExecutionResultDigest
	sums     []MemberSummary
	dirty    []bool
	running  int // position of the running member, -1 for none
	counts   MemberCounts
	attn     int
}

// newMemberFold folds the session's current persisted state. A session's
// members are those in its scope that own at least one frozen component, in the
// order of their first component.
func (s *serviceImpl) newMemberFold(e *PlanExecution, units []ExecutionUnit) (*memberFold, error) {
	members, err := s.repo.ListWorksetMembers(e.WorksetID)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load members", err)
	}
	digests, err := s.repo.ListExecutionResultDigests(e.ExecutionID, 0)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load component results", err)
	}
	// Ownership is the frozen plan root's member: a rename moves the member's
	// folder path while its session runs, so a live path match alone would lose
	// it. The live path still places units whose root the plan does not own.
	frame, err := s.repo.GetPlanFrame(e.PlanID)
	if err != nil {
		return nil, NewError(ErrKindInternal, "INTERNAL", "failed to load plan roots", err)
	}
	ownerOfRoot := make(map[int]string, len(frame.Roots))
	for _, r := range frame.Roots {
		if r.MemberID != "" {
			ownerOfRoot[r.RootIndex] = r.MemberID
		}
	}
	byRoot := make(map[string]*WorksetMember, len(members))
	for _, m := range members {
		byRoot[m.FolderPath] = m
	}
	f := &memberFold{
		owner:    make(map[int]int, len(units)),
		byComp:   make(map[string]int, len(units)),
		byMember: make(map[string]int),
		digests:  make(map[int]ExecutionResultDigest, len(digests)),
		running:  -1,
	}
	for _, u := range units {
		memberID, owned := ownerOfRoot[u.RootIndex]
		if !owned {
			m := byRoot[u.RootPath]
			if m == nil {
				continue
			}
			memberID = m.MemberID
		}
		pos, ok := f.byMember[memberID]
		if !ok {
			pos = len(f.recs)
			f.byMember[memberID] = pos
			f.recs = append(f.recs, memberRec{id: memberID, root: u.RootPath})
		}
		f.recs[pos].idx = append(f.recs[pos].idx, u.Index)
		f.recs[pos].ops += u.Operations
		f.owner[u.Index] = pos
		f.byComp[u.ID] = pos
	}
	for _, d := range digests {
		if _, ok := f.owner[d.ComponentIndex]; ok {
			f.digests[d.ComponentIndex] = d
		}
	}
	f.setRunningPos(e)
	f.sums = make([]MemberSummary, len(f.recs))
	f.dirty = make([]bool, len(f.recs))
	for i := range f.recs {
		f.sums[i] = f.summarize(i)
		f.account(f.sums[i], 1)
	}
	return f, nil
}

func sessionLive(status string) bool {
	return status == ExecStatusQueued || status == ExecStatusRunning
}

// setRunningPos points running at the member holding the session's current
// component, only while the session is live.
func (f *memberFold) setRunningPos(e *PlanExecution) {
	f.running = -1
	if sessionLive(e.Status) {
		if pos, ok := f.byComp[e.CurrentComponentID]; ok {
			f.running = pos
		}
	}
}

func (f *memberFold) summarize(i int) MemberSummary {
	rec := f.recs[i]
	sum := MemberSummary{MemberID: rec.id, RootPath: rec.root, TotalComponents: len(rec.idx), TotalOperations: rec.ops}
	var succeeded, failed, canceled int
	haveErr := false
	for _, ci := range rec.idx {
		d, ok := f.digests[ci]
		if !ok {
			continue
		}
		status, code, msg := d.Status, d.ErrorCode, d.ErrorMessage
		if d.Unreadable {
			status, code, msg = ExecComponentFailed, "RESULT_UNREADABLE", "the recorded component result could not be read"
		}
		sum.CompletedOperations += d.CompletedOperations
		if status != ExecComponentPending {
			sum.CompletedComponents++
		}
		switch status {
		case ExecComponentSucceeded:
			succeeded++
		case ExecComponentFailed:
			failed++
			if !haveErr {
				haveErr = true
				sum.ErrorMessage = msg
				if msg == "" {
					sum.ErrorMessage = code
				}
			}
		case ExecComponentCanceled:
			canceled++
		}
		if sum.InventorySyncError == "" {
			sum.InventorySyncError = d.InventorySyncError
		}
	}
	sum.Status = memberStatus(f.running == i, len(rec.idx), succeeded, failed, canceled)
	return sum
}

func (f *memberFold) account(s MemberSummary, sign int) {
	switch s.Status {
	case MemberSucceeded:
		f.counts.Succeeded += sign
	case MemberFailed:
		f.counts.Failed += sign
	case MemberCanceled:
		f.counts.Canceled += sign
	case MemberRunning:
		f.counts.Running += sign
	default:
		f.counts.Pending += sign
	}
	if isAttention(s) {
		f.attn += sign
	}
}

func (f *memberFold) recompute(i int) {
	next := f.summarize(i)
	if next == f.sums[i] {
		return
	}
	f.account(f.sums[i], -1)
	f.account(next, 1)
	f.sums[i] = next
	f.dirty[i] = true
}

// land folds a result that just appeared.
func (f *memberFold) land(d ExecutionResultDigest) {
	pos, ok := f.owner[d.ComponentIndex]
	if !ok {
		return
	}
	f.digests[d.ComponentIndex] = d
	f.recompute(pos)
}

// move follows the session row: the running member changes with the current
// component, so both the member left and the member entered are recomputed.
func (f *memberFold) move(e *PlanExecution) {
	old := f.running
	f.setRunningPos(e)
	if old == f.running {
		return
	}
	for _, pos := range []int{old, f.running} {
		if pos >= 0 {
			f.recompute(pos)
		}
	}
}

// drain returns an event for each member whose summary changed since the last
// drain, in member order, each carrying the counts as they stand now.
func (f *memberFold) drain() []MemberEvent {
	var out []MemberEvent
	for i, d := range f.dirty {
		if !d {
			continue
		}
		f.dirty[i] = false
		out = append(out, MemberEvent{Member: f.sums[i], Counts: f.counts, AttentionTotal: f.attn})
	}
	return out
}

func (f *memberFold) view() *ExecutionMembers {
	out := &ExecutionMembers{Counts: f.counts, Attention: []MemberSummary{}, AttentionTotal: f.attn}
	for _, sum := range f.sums {
		if len(out.Attention) == attentionCap {
			break
		}
		if isAttention(sum) {
			out.Attention = append(out.Attention, sum)
		}
	}
	return out
}

// memberUnits returns one member's frozen units and the result-row range that
// covers them (first index, row limit).
func (f *memberFold) memberUnits(pos int, units []ExecutionUnit) ([]ExecutionUnit, int, int) {
	mine := make([]ExecutionUnit, 0, len(f.recs[pos].idx))
	for _, u := range units {
		if owner, ok := f.owner[u.Index]; ok && owner == pos {
			mine = append(mine, u)
		}
	}
	first, last := mine[0].Index, mine[len(mine)-1].Index
	return mine, first, last - first + 1
}

// SubscribeExecutionMembers is SubscribeExecution for view=members: the
// snapshot is the members summary, and instead of `component` events a
// `member` event goes out for each member whose summary changed, folded in
// memory from the identity the connection holds. Within a tick the member
// events precede progress, and the terminal event is last.
func (s *serviceImpl) SubscribeExecutionMembers(
	ctx context.Context,
	worksetID, operationType, executionID string,
	emit func(event string, data any) error,
) error {
	e, err := s.loadExecution(worksetID, operationType, executionID)
	if err != nil {
		return err
	}
	req, err := parseExecutionRequest(e.RequestJSON)
	if err != nil {
		return NewError(ErrKindInternal, "REQUEST_LOAD_FAILED", "failed to load the frozen request", err)
	}
	fold, err := s.newMemberFold(e, req.Units)
	if err != nil {
		return err
	}
	snapshot, err := s.executionViewOf(e, ExecutionRead{Members: true})
	if err != nil {
		return err
	}
	if emitErr := emit(ExecutionEventSnapshot, snapshot); emitErr != nil {
		return emitErr
	}
	if executionTerminal(e.Status) {
		return emitExecutionTerminal(emit, e)
	}

	// ponytail: unordered (batch) streams reread digests from index 0 each tick;
	// members mode is conversion's, which commits in order.
	_, unordered := s.batchExecutors[operationType]
	cursor := 0
	for idx := range fold.digests {
		if !unordered {
			cursor = max(cursor, idx+1)
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	last := progressOf(e)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		cur, loadErr := s.repo.GetExecutionProgress(executionID)
		if loadErr != nil {
			return nil // disconnect fallback: client reconnects to the detail route
		}
		digests, listErr := s.repo.ListExecutionResultDigests(executionID, cursor)
		if listErr != nil {
			return nil
		}
		fold.move(cur)
		for _, d := range digests {
			if _, done := fold.digests[d.ComponentIndex]; done {
				continue
			}
			fold.land(d)
			if !unordered {
				cursor = d.ComponentIndex + 1
			}
		}
		for _, ev := range fold.drain() {
			if emitErr := emit(ExecutionEventMember, ev); emitErr != nil {
				return emitErr
			}
		}
		if next := progressOf(cur); next != last {
			last = next
			if emitErr := emit(ExecutionEventProgress, next); emitErr != nil {
				return emitErr
			}
		}
		if executionTerminal(cur.Status) {
			return emitExecutionTerminal(emit, cur)
		}
	}
}
