package workset_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Clementine33/preen/backend/internal/workset"
)

// logSink is a goroutine-safe buffer: the dispatcher logs from its own goroutine.
type logSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// captureLogs routes the process logger to a JSON sink for one test; SetDefault
// also redirects the log package, so both are restored.
func captureLogs(t *testing.T) *logSink {
	t.Helper()
	sink := &logSink{}
	previous, output, flags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(sink, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous); log.SetOutput(output); log.SetFlags(flags) })
	return sink
}

// lines returns the records with the given msg.
func (s *logSink) lines(t *testing.T, msg string) []map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for raw := range strings.SplitSeq(strings.TrimSpace(s.buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			t.Fatalf("log line %q: %v", raw, err)
		}
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

func (s *logSink) one(t *testing.T, msg string) map[string]any {
	t.Helper()
	got := s.lines(t, msg)
	if len(got) != 1 {
		t.Fatalf("%q logged %d times, want once: %v", msg, len(got), got)
	}
	return got[0]
}

func wantField(t *testing.T, rec map[string]any, key string, want any) {
	t.Helper()
	if rec[key] != want {
		t.Fatalf("%s = %v, want %v (record %v)", key, rec[key], want, rec)
	}
}

// A unit that fails on its own is logged once as a component and once as the
// session it stopped, each with its own identifiers.
func TestExecutionFailureIsLoggedOncePerUnitAndSession(t *testing.T) {
	logs := captureLogs(t)
	refused := poolUnit(0)
	refused.FailPrepare = true
	f, _ := poolFixture(t, 1, refused)
	f.seedRevision("plan-log", poolComponent("u0", "albumA"))

	started := f.mustStart("plan-log", "k-log")
	f.waitTerminal(started.ExecutionID)

	unit := logs.one(t, "execution component failed")
	wantField(t, unit, "execution_id", started.ExecutionID)
	wantField(t, unit, "plan_id", "plan-log")
	wantField(t, unit, "operation_type", workset.OperationTypeConversion)
	wantField(t, unit, "component_id", "u0")
	wantField(t, unit, "stage", "precheck")
	wantField(t, unit, "error_code", "PREPARE_REFUSED")
	session := logs.one(t, "execution failed")
	wantField(t, session, "execution_id", started.ExecutionID)
	wantField(t, session, "error_code", "PREPARE_REFUSED")
	if _, ok := session["workset_id"]; !ok {
		t.Fatalf("session line lacks workset_id: %v", session)
	}
}

func TestExecutionFailureIsLoggedBeforeTerminalWriteFails(t *testing.T) {
	logs := captureLogs(t)
	plan := poolUnit(0)
	plan.FailPrepare = true
	f, _ := poolFixture(t, 1, plan)
	f.seedRevision("plan-terminal-write", poolComponent("u0", "albumA"))
	if _, err := f.repo.DB().Exec(`
		CREATE TRIGGER fail_execution_terminal_write
		BEFORE UPDATE OF status ON plan_executions
		WHEN NEW.status = 'failed'
		BEGIN SELECT RAISE(FAIL, 'terminal write blocked'); END
	`); err != nil {
		t.Fatal(err)
	}
	started := f.mustStart("plan-terminal-write", "k-terminal-write")
	deadline := time.After(4 * time.Second)
	for len(logs.lines(t, "execution terminal write failed")) == 0 {
		select {
		case <-deadline:
			t.Fatal("terminal write failure was not logged")
		case <-time.After(10 * time.Millisecond):
		}
	}
	wantField(t, logs.one(t, "execution failed"), "execution_id", started.ExecutionID)
	logs.mu.Lock()
	output := logs.buf.String()
	logs.mu.Unlock()
	failureAt := strings.Index(output, `"msg":"execution failed"`)
	writeErrorAt := strings.Index(output, `"msg":"execution terminal write failed"`)
	if failureAt >= writeErrorAt {
		t.Fatalf("failure log followed terminal write error: %s", output)
	}
}

// The user's stop is an outcome, not a failure.
func TestExecutionCancellationIsNotLogged(t *testing.T) {
	logs := captureLogs(t)
	f, obs := poolFixture(t, 1, poolUnit(1))
	obs.gate("u0")
	f.seedRevision("plan-cancel-log", poolComponent("u0", "albumA"))

	started := f.mustStart("plan-cancel-log", "k-cancel-log")
	obs.waitEncode(t, "u0")
	if _, err := f.svc.CancelExecution(
		f.t.Context(), f.worksetID, workset.OperationTypeConversion, started.ExecutionID,
	); err != nil {
		t.Fatalf("CancelExecution: %v", err)
	}
	f.waitTerminal(started.ExecutionID)

	for _, msg := range []string{
		"execution failed", "execution component failed", "execution canceled without a cancel request",
	} {
		if got := logs.lines(t, msg); len(got) != 0 {
			t.Fatalf("%q logged on a user cancel: %v", msg, got)
		}
	}
}

// A deadline-shaped error with no cancel request is a real failure read as a
// cancel; the warning must fire wherever the error is classified, whichever of
// prepare, encode or commit returned it.
func TestExecutionUnrequestedCancelIsWarned(t *testing.T) {
	cases := map[string]func(*workset.UnitRunInput, *poolPlan){
		"prepare": func(_ *workset.UnitRunInput, p *poolPlan) { p.PrepareDeadline = true },
		"encode":  func(_ *workset.UnitRunInput, p *poolPlan) { p.EncodeDeadline = true },
		"commit":  func(_ *workset.UnitRunInput, p *poolPlan) { p.CommitDeadline = true },
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureLogs(t)
			plan := poolUnit(1)
			script(nil, &plan)
			f, _ := poolFixture(t, 1, plan)
			f.seedRevision("plan-deadline", poolComponent("u0", "albumA"))

			started := f.mustStart("plan-deadline", "k-deadline")
			f.waitTerminal(started.ExecutionID)

			warn := logs.one(t, "execution canceled without a cancel request")
			wantField(t, warn, "execution_id", started.ExecutionID)
			if got, _ := warn["err"].(string); !strings.Contains(got, "deadline exceeded") {
				t.Fatalf("err = %v, want the original deadline error", warn["err"])
			}
		})
	}
}

// err carries the business error's cause, and is left out once the message
// already says it.
func TestExecutionFailureLogCarriesCauseOnlyWhenNew(t *testing.T) {
	for name, tc := range map[string]struct {
		inMessage bool
		wantErr   bool
	}{"new cause": {false, true}, "cause already in message": {true, false}} {
		t.Run(name, func(t *testing.T) {
			logs := captureLogs(t)
			plan := poolUnit(0)
			plan.PrepareCause, plan.PrepareCauseInMessage = "disk detail", tc.inMessage
			f, _ := poolFixture(t, 1, plan)
			f.seedRevision("plan-cause", poolComponent("u0", "albumA"))

			started := f.mustStart("plan-cause", "k-cause")
			f.waitTerminal(started.ExecutionID)

			session := logs.one(t, "execution failed")
			got, has := session["err"].(string)
			if has != tc.wantErr || (has && got != "disk detail") {
				t.Fatalf("err = %v, want present=%v", session["err"], tc.wantErr)
			}
		})
	}
}

// A plain Go error is kept as it is: the generation's message is generic, so the
// scan's own error must reach the log.
func TestGenerationFailureIsLoggedWithPlainCause(t *testing.T) {
	logs := captureLogs(t)
	f := newFixtureWithScan(t, func(context.Context, string, string) error {
		return errors.New("scan exploded")
	})
	ids := f.standardLibrary("albumA")
	f.insertAudioEntry("/music/albumA/01.mp3", "/music", 1024, 1000)
	ws := f.createCurrent("扫描失败", ids...)

	gen := f.runGeneration(ws.WorksetID, f.operation(ws.WorksetID).Version)
	if gen.Status != "failed" {
		t.Fatalf("generation = %+v", gen)
	}
	rec := logs.one(t, "generation failed")
	wantField(t, rec, "generation_id", gen.GenerationID)
	wantField(t, rec, "workset_id", ws.WorksetID)
	wantField(t, rec, "operation_type", workset.OperationTypeConversion)
	wantField(t, rec, "error_code", "SCAN_FAILED")
	wantField(t, rec, "err", "scan exploded")
}

// Units a batch never ran take the session's outcome; they are summed on the
// session line, not logged one by one.
func TestBatchFailureSummarizesUnrunUnits(t *testing.T) {
	logs := captureLogs(t)
	obs := newPoolObservations()
	task := newPoolTask(obs, poolUnit(0), poolUnit(0), poolUnit(0))
	batch := map[string]workset.BatchExecutor{
		workset.OperationTypeConversion: func(_ context.Context, in workset.BatchExecutionInput) error {
			zero := 0
			if err := in.Progress(in.Units[0].Index, workset.UnitResult{CompletedOperations: &zero}); err != nil {
				return err
			}
			return errors.New("batch exploded")
		},
	}
	f := newExecFixtureWithTask(t, nil, 1, []workset.Task{task}, batch)
	f.runDispatcher()
	f.seedRevision(
		"plan-batch",
		poolComponent("u0", "albumA"),
		poolComponent("u1", "albumA"),
		poolComponent("u2", "albumB"),
	)

	started := f.mustStart("plan-batch", "k-batch")
	f.waitTerminal(started.ExecutionID)

	if got := logs.lines(t, "execution component failed"); len(got) != 0 {
		t.Fatalf("rewritten units were logged one by one: %v", got)
	}
	session := logs.one(t, "execution failed")
	wantField(t, session, "pending_count", float64(2))
	wantField(t, session, "error_message", "batch exploded")
	if _, has := session["err"]; has {
		t.Fatalf("err repeats the message: %v", session)
	}
}
