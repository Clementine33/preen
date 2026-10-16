package workset_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/conversion"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
	"github.com/onsei/organizer/backend/internal/workset"
)

// elsewhere is a complete common configuration that differs from draftDoc()
// in every one of its six values, so saving it is unambiguously a
// common-settings change.
func elsewhere(tags ...string) *conversion.DraftDoc {
	doc := draftDoc()
	doc.Mode = reconcile.ModeStrict
	doc.DeleteMode = "hard"
	doc.NoEffectTags = tags
	doc.OtherTags = []string{"revert"}
	doc.Matched = reconcile.DesiredProfile{
		Lossless: &reconcile.AudioOutputSpec{Codec: reconcile.CodecFlac},
		Encoded: &reconcile.AudioOutputSpec{
			Codec:   reconcile.CodecAac,
			Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 256},
		},
	}
	doc.Unmatched = reconcile.DesiredProfile{
		Encoded: &reconcile.AudioOutputSpec{
			Codec:   reconcile.CodecMp3,
			Quality: &reconcile.Quality{Kind: reconcile.QualityBitrate, Bitrate: 192},
		},
	}
	return doc
}

// twoLibraries builds one record in each of two libraries, so a global save has
// a record to reach beyond the one it came through.
func (f *fixture) twoLibraries(t *testing.T) (*workset.WorksetView, *workset.WorksetView) {
	t.Helper()
	f.standardLibrary("albumA", "albumB")
	f.insertLibrary("lib-2", "/music2")
	f.insertDir("/music2", "albumC", true)
	first := f.createCurrent("一库", "albumA", "albumB")
	second := f.createCurrentFor("lib-2", "二库", "albumC")
	return first, second
}

// stamp is a hand-written session row's timestamp.
func stamp() string { return time.Now().Format(timeFmt) }

// reopen drops the fixture's repository and service and builds them again over
// the same data directory: what a test sees next is a restarted process's view
// of the database.
func (f *fixture) reopen() {
	f.t.Helper()
	if err := f.repo.Close(); err != nil {
		f.t.Fatalf("close repo: %v", err)
	}
	f.open()
}

// saveSettings publishes a conversion document's common section through the
// Task settings seam, echoing the ETag it just read.
func (f *fixture) saveSettings(doc *conversion.DraftDoc) *workset.TaskSettings {
	f.t.Helper()
	current, err := f.svc.GetTaskSettings(f.ctx, workset.OperationTypeConversion)
	if err != nil {
		f.t.Fatalf("GetTaskSettings: %v", err)
	}
	saved, err := f.trySaveSettings(doc, current.ETag)
	if err != nil {
		f.t.Fatalf("SaveTaskSettings: %v", err)
	}
	return saved
}

func (f *fixture) trySaveSettings(doc *conversion.DraftDoc, etag string) (*workset.TaskSettings, error) {
	f.t.Helper()
	payload, err := json.Marshal(conversion.ProjectCommonSettings(doc))
	if err != nil {
		f.t.Fatalf("marshal settings: %v", err)
	}
	return f.svc.SaveTaskSettings(f.ctx, workset.OperationTypeConversion, payload, etag)
}

// TestTaskSettingsReadSeedThenStored covers the endpoint's read side: with
// nothing saved it serves the Task's seed value, and a save changes the value
// and the ETag. It needs no record.
func TestTaskSettingsReadSeedThenStored(t *testing.T) {
	f := newFixture(t)
	seed, err := f.svc.GetTaskSettings(f.ctx, workset.OperationTypeConversion)
	if err != nil || seed.ETag == "" {
		t.Fatalf("seed settings = %+v err=%v", seed, err)
	}
	if common, getErr := f.repo.GetCommonSettings(workset.OperationTypeConversion); getErr != nil || common != nil {
		t.Fatalf("reading stored settings: %+v err=%v", common, getErr)
	}

	saved := f.saveSettings(elsewhere("反転"))
	if saved.ETag == seed.ETag {
		t.Fatal("a changed value kept its ETag")
	}
	got, err := f.svc.GetTaskSettings(f.ctx, workset.OperationTypeConversion)
	if err != nil || got.ETag != saved.ETag || string(got.Settings) != string(saved.Settings) {
		t.Fatalf("stored settings = %+v err=%v, want %+v", got, err, saved)
	}
	if !strings.Contains(string(got.Settings), `"strict"`) {
		t.Fatalf("stored settings = %s", got.Settings)
	}
}

// TestInvalidTaskSettingsAreRefused covers the write side's validation: a
// payload the task cannot read changes nothing.
func TestInvalidTaskSettingsAreRefused(t *testing.T) {
	f := newFixture(t)
	seed, _ := f.svc.GetTaskSettings(f.ctx, workset.OperationTypeConversion)
	_, err := f.svc.SaveTaskSettings(
		f.ctx, workset.OperationTypeConversion, json.RawMessage(`{"mode":`), seed.ETag,
	)
	if werr, ok := workset.AsError(err); !ok || werr.Kind != workset.ErrKindInvalidArgument {
		t.Fatalf("want an invalid-argument refusal, got %v", err)
	}
	if common, getErr := f.repo.GetCommonSettings(workset.OperationTypeConversion); getErr != nil || common != nil {
		t.Fatalf("a refused save stored settings: %+v err=%v", common, getErr)
	}
}

// TestCommonSettingsSaveReachesEveryRecord covers the deployment-wide rule: a
// settings save stores them once and rewrites every editable record's common
// fields with them — its member overrides and
// exclusions untouched, its operation version advanced.
func TestCommonSettingsSaveReachesEveryRecord(t *testing.T) {
	f := newFixture(t)
	first, second := f.twoLibraries(t)

	// The other record carries a member override and an excluded member: the
	// common change must reach its common fields and nothing else.
	strict := reconcile.ModeStrict
	other := mustDraft(t, f.draft(second.WorksetID))
	emptyOther := []string{}
	other.Members = []conversion.DraftMember{{
		MemberID:  second.Members[0].MemberID,
		Excluded:  true,
		Overrides: &conversion.OverrideSet{Mode: &strict, OtherTags: &emptyOther},
	}}
	before := f.saveDraft(second.WorksetID, other, second.Operations[0].Version)

	f.saveSettings(elsewhere("SEあり", "反転"))
	if got := f.operation(first.WorksetID).Version; got != first.Operations[0].Version+1 {
		t.Fatalf("first record version = %d, want %d", got, first.Operations[0].Version+1)
	}

	synced := mustDraft(t, f.draft(second.WorksetID))
	want := elsewhere("SEあり", "反転")
	if synced.Mode != want.Mode || synced.DeleteMode != want.DeleteMode {
		t.Fatalf("synced mode/delete_mode = %q/%q, want %q/%q",
			synced.Mode, synced.DeleteMode, want.Mode, want.DeleteMode)
	}
	if len(synced.NoEffectTags) != 2 || synced.NoEffectTags[0] != "SEあり" {
		t.Fatalf("synced tags = %v, want %v", synced.NoEffectTags, want.NoEffectTags)
	}
	if synced.Matched.Lossless == nil || synced.Matched.Lossless.Codec != reconcile.CodecFlac {
		t.Fatalf("synced matched = %+v, want the new common profile", synced.Matched)
	}
	if len(synced.Members) != 1 {
		t.Fatalf("member records = %+v, want the stored one", synced.Members)
	}
	member := synced.Members[0]
	if !member.Excluded || member.Overrides == nil || member.Overrides.Mode == nil ||
		*member.Overrides.Mode != reconcile.ModeStrict {
		t.Fatalf("the synced member record lost its override or exclusion: %+v", member)
	}
	// The override is still the only thing the member carries: a common
	// rewrite never materializes a member record, it only moves the base.
	if member.Overrides.Matched != nil || member.Overrides.NoEffectTags != nil || member.Overrides.OtherTags == nil ||
		len(*member.Overrides.OtherTags) != 0 {
		t.Fatalf("the common rewrite touched units the member never overrode: %+v", member.Overrides)
	}
	if after := f.operation(second.WorksetID); after.Version != before.Version+1 {
		t.Fatalf("synced record version = %d, want %d", after.Version, before.Version+1)
	}
}

// TestCommonSettingsSaveLeavesGeneratedPlansAndNeedsReplanning covers what a
// global save does to plans: the plan a record already holds is a frozen
// snapshot and stays exactly as it was, while the record's settings are now
// ahead of it — so the operation reads as needing planning, and executing the
// old plan is refused until a new one is generated (ADR 0008).
func TestCommonSettingsSaveLeavesGeneratedPlansAndNeedsReplanning(t *testing.T) {
	f := newFixture(t)
	_, second := f.twoLibraries(t)
	f.insertAudioEntry("/music2/albumC/01.mp3", "/music2/albumC", 1024, 1000)

	planned := f.saveDraft(second.WorksetID, mustDraft(t, f.draft(second.WorksetID)), second.Operations[0].Version)
	gen := f.runGeneration(second.WorksetID, planned.Version)
	if gen.Status != workset.GenStatusCompleted || gen.RevisionID == "" {
		t.Fatalf("generation = %+v", gen)
	}
	planID := f.operation(second.WorksetID).CurrentRevision.PlanID

	f.saveSettings(elsewhere("SEなし"))

	op := f.operation(second.WorksetID)
	if op.PlanningState != workset.PlanningNeedsPlanning {
		t.Fatalf("planning state = %q, want %q", op.PlanningState, workset.PlanningNeedsPlanning)
	}
	if op.CurrentRevision.PlanID != planID {
		t.Fatalf("the synced record's plan changed: %q -> %q", planID, op.CurrentRevision.PlanID)
	}
	revision, err := f.svc.GetRevision(
		f.ctx, second.WorksetID, workset.OperationTypeConversion, planID,
	)
	if err != nil {
		t.Fatalf("the frozen plan must stay reviewable: %v", err)
	}
	if revision.Plan.SnapshotToken == "" || len(revision.Members) != 1 {
		t.Fatalf("frozen plan = %+v", revision.Plan)
	}
	_, execErr := f.svc.StartExecution(
		f.ctx, second.WorksetID, workset.OperationTypeConversion, planID,
		workset.StartExecutionRequest{IfMatchVersion: op.Version, IdempotencyKey: "k-after-common"},
	)
	if execErr == nil {
		t.Fatal("executing a plan the common settings outran must be refused")
	}
	if werr, ok := workset.AsError(execErr); !ok || werr.Code != "PLAN_NOT_EXECUTABLE" {
		t.Fatalf("want PLAN_NOT_EXECUTABLE, got %v", execErr)
	}
}

// TestMemberOnlySaveStaysInsideItsRecord covers the other half of the rule: a
// save that changes only member records never touches the deployment or its
// neighbours.
func TestMemberOnlySaveStaysInsideItsRecord(t *testing.T) {
	f := newFixture(t)
	first, second := f.twoLibraries(t)

	before := f.operation(second.WorksetID)
	beforeDraft := f.draft(second.WorksetID)

	tags := []string{"SEなし"}
	doc := mustDraft(t, f.draft(first.WorksetID))
	doc.Members = []conversion.DraftMember{{
		MemberID:  first.Members[0].MemberID,
		Overrides: &conversion.OverrideSet{NoEffectTags: &tags},
	}}
	saved := f.saveDraft(first.WorksetID, doc, first.Operations[0].Version)

	if saved.Version != first.Operations[0].Version+1 {
		t.Fatalf("own operation version = %d", saved.Version)
	}
	if after := f.operation(second.WorksetID); after.Version != before.Version {
		t.Fatalf("a member-only save moved another record's version: %d -> %d",
			before.Version, after.Version)
	}
	if got := f.draft(second.WorksetID); string(got.Document) != string(beforeDraft.Document) {
		t.Fatalf("another record's draft changed: %s -> %s", beforeDraft.Document, got.Document)
	}
	if common, err := f.repo.GetCommonSettings(workset.OperationTypeConversion); err != nil || common != nil {
		t.Fatalf("a member-only save stored deployment-wide settings: %+v err=%v", common, err)
	}
}

// TestCommonSettingsSaveRefusedWhileARecordIsBusy covers the all-or-nothing
// rule: one record busy with a planning session or an execution refuses the
// whole save, and nothing is written.
func TestCommonSettingsSaveRefusedWhileARecordIsBusy(t *testing.T) {
	for _, reason := range []string{workset.RefusalGenerating, workset.RefusalExecuting} {
		t.Run(reason, func(t *testing.T) {
			f := newFixture(t)
			first, second := f.twoLibraries(t)
			f.occupy(second.WorksetID, reason)
			firstBefore := f.operation(first.WorksetID)
			busyBefore := f.operation(second.WorksetID)
			current, _ := f.svc.GetTaskSettings(f.ctx, workset.OperationTypeConversion)

			_, err := f.trySaveSettings(elsewhere("SEなし"), current.ETag)
			if werr, ok := workset.AsError(err); !ok || werr.Code != "COMMON_SETTINGS_BUSY" {
				t.Fatalf("want COMMON_SETTINGS_BUSY, got %v", err)
			}
			if got := f.operation(first.WorksetID).Version; got != firstBefore.Version {
				t.Fatalf("the refused save advanced a record: %d -> %d", firstBefore.Version, got)
			}
			if got := f.operation(second.WorksetID).Version; got != busyBefore.Version {
				t.Fatalf("the refused save advanced the busy record: %d -> %d", busyBefore.Version, got)
			}
			if common, getErr := f.repo.GetCommonSettings(
				workset.OperationTypeConversion,
			); getErr != nil || common != nil {
				t.Fatalf("a refused save stored settings: %+v err=%v", common, getErr)
			}
		})
	}
}

// occupy puts a record into a queued or running session by hand: the store's
// busy rule reads these two tables, and a real session would race the
// assertion that follows.
func (f *fixture) occupy(worksetID, reason string) {
	f.t.Helper()
	f.exec(`
		INSERT INTO plan_generations (generation_id, workset_id, operation_type, status, created_at, updated_at)
		VALUES ('gen-busy', ?, 'conversion', 'running', ?, ?)
	`, worksetID, stamp(), stamp())
	if reason == workset.RefusalExecuting {
		f.exec(`UPDATE plan_generations SET status = 'completed' WHERE generation_id = 'gen-busy'`)
		f.exec(`
			INSERT INTO plan_executions (execution_id, workset_id, operation_type, plan_id, status, created_at, updated_at)
			VALUES ('exec-busy', ?, 'conversion', 'plan-busy', 'running', ?, ?)
		`, worksetID, stamp(), stamp())
	}
}

func TestCommonSettingsSaveVersionConflictChangesNothing(t *testing.T) {
	f := newFixture(t)
	_, second := f.twoLibraries(t)
	stale, _ := f.svc.GetTaskSettings(f.ctx, workset.OperationTypeConversion)
	f.saveSettings(draftDoc())
	f.saveSettings(elsewhere("SEなし"))

	before := f.operation(second.WorksetID)
	_, err := f.trySaveSettings(elsewhere("反転"), stale.ETag)
	if werr, ok := workset.AsError(err); !ok || werr.Code != "VERSION_CONFLICT" {
		t.Fatalf("want VERSION_CONFLICT, got %v", err)
	}
	after := f.operation(second.WorksetID)
	if after.Version != before.Version || after.PlanningState != before.PlanningState {
		t.Fatalf("the refused save moved another record: %+v -> %+v", before, after)
	}
}

// TestCommonSettingsSeedNewRecordsAndSurviveRestart covers the stored payload:
// a record created after a global save starts from it, and it is still there
// after the process is restarted on the same data directory.
func TestCommonSettingsSeedNewRecordsAndSurviveRestart(t *testing.T) {
	f := newFixture(t)
	f.twoLibraries(t)

	f.saveSettings(elsewhere("反転"))

	seeded := mustDraft(t, f.draft(f.createCurrent("新建", "albumA").WorksetID))
	if seeded.Mode != reconcile.ModeStrict || seeded.DeleteMode != "hard" {
		t.Fatalf("a new record seeded mode/delete_mode = %q/%q", seeded.Mode, seeded.DeleteMode)
	}
	if len(seeded.NoEffectTags) != 1 || seeded.NoEffectTags[0] != "反転" {
		t.Fatalf("a new record seeded tags = %v", seeded.NoEffectTags)
	}
	if seeded.Matched.Encoded == nil || seeded.Matched.Encoded.Codec != reconcile.CodecAac {
		t.Fatalf("a new record seeded matched = %+v", seeded.Matched)
	}

	f.reopen()
	restarted := mustDraft(t, f.draft(f.createCurrent("重启", "albumB").WorksetID))
	if restarted.Mode != reconcile.ModeStrict || len(restarted.NoEffectTags) != 1 ||
		restarted.NoEffectTags[0] != "反転" {
		t.Fatalf("after a restart the stored settings seeded %+v", restarted)
	}
}

// TestCommonSettingsSkipOrphanedRecords covers the read-only rule: a record
// whose library is gone never takes part in a global save.
func TestCommonSettingsSkipOrphanedRecords(t *testing.T) {
	f := newFixture(t)
	_, second := f.twoLibraries(t)
	// The delete path removes a record rather than orphaning it; this is the
	// state a record left behind by a vanished library row is in.
	f.exec("UPDATE worksets SET library_id = NULL WHERE id = ?", second.WorksetID)

	before := f.operation(second.WorksetID)
	beforeDraft := f.draft(second.WorksetID)
	f.saveSettings(elsewhere("SEなし"))

	after := f.operation(second.WorksetID)
	if after.Version != before.Version || after.PlanningState != workset.PlanningOrphaned {
		t.Fatalf("the orphaned record moved: %+v -> %+v", before, after)
	}
	if got := f.draft(second.WorksetID); string(got.Document) != string(beforeDraft.Document) {
		t.Fatalf("the orphaned record's draft was rewritten: %s", got.Document)
	}
}

// Without a global row, one record's old settings cannot silently become a
// different record's defaults. The user chooses the source by applying the
// settings from that record.
func TestCreateDoesNotGuessGlobalSettingsFromAnotherRecord(t *testing.T) {
	f := newFixture(t)
	ids := f.standardLibrary("albumA")
	configured := f.createCurrent("旧设置", ids...)
	f.exec(
		"UPDATE workset_operation_drafts SET draft_json = ?, updated_at = ? WHERE workset_id = ?",
		string(draftJSON(t, elsewhere("反転"))), stamp(), configured.WorksetID,
	)
	f.insertLibrary("lib-2", "/music2")
	f.insertDir("/music2", "albumC", true)
	fresh := f.createCurrentFor("lib-2", "未配置", "albumC")
	seeded := mustDraft(t, f.draft(fresh.WorksetID))
	if seeded.Mode != reconcile.ModeAvailableSources || seeded.DeleteMode != "" ||
		seeded.Unmatched.Encoded == nil || seeded.Unmatched.Encoded.Quality == nil ||
		seeded.Unmatched.Encoded.Quality.Bitrate != 320 {
		t.Fatalf("an unrelated record inherited unsaved common settings: %+v", seeded)
	}
	if common, err := f.repo.GetCommonSettings(workset.OperationTypeConversion); err != nil || common != nil {
		t.Fatalf("creation must not guess a deployment-wide value: %+v err=%v", common, err)
	}
}

// Settings equal to the seed still become the stored value: it is the user's
// choice, not whatever the seed is next time.
func TestSavingTheSeedValuePublishesIt(t *testing.T) {
	f := newFixture(t)
	f.twoLibraries(t)
	f.saveSettings(draftDoc())
	if common, err := f.repo.GetCommonSettings(workset.OperationTypeConversion); err != nil || common == nil {
		t.Fatalf("common settings were not stored: %+v err=%v", common, err)
	}
}

// An older database has no global row. A later member-only save in another
// record must not turn that record's unchanged defaults into the global choice
// when the user replaces the record whose common settings they configured.
func TestReplaceKeepsCurrentRecordSettingsWithoutGlobalRow(t *testing.T) {
	f := newFixture(t)
	first, second := f.twoLibraries(t)
	f.exec("DELETE FROM common_settings")
	f.exec(
		"UPDATE workset_operation_drafts SET draft_json = ?, updated_at = ? WHERE workset_id = ?",
		string(draftJSON(t, elsewhere("反転"))),
		time.Now().Add(time.Minute).Format(timeFmt), first.WorksetID,
	)
	memberOnly := draftDoc()
	memberOnly.Members = []conversion.DraftMember{{MemberID: second.Members[0].MemberID, Excluded: true}}
	f.exec(
		"UPDATE workset_operation_drafts SET draft_json = ?, updated_at = ? WHERE workset_id = ?",
		string(draftJSON(t, memberOnly)),
		time.Now().Add(2*time.Minute).Format(timeFmt), second.WorksetID,
	)

	replaced := f.createCurrent("换目录", "albumB")
	seeded := mustDraft(t, f.draft(replaced.WorksetID))
	if seeded.Unmatched.Encoded == nil || seeded.Unmatched.Encoded.Codec != reconcile.CodecMp3 ||
		seeded.Unmatched.Encoded.Quality == nil || seeded.Unmatched.Encoded.Quality.Bitrate != 192 {
		t.Fatalf("replacement lost the current record's common settings: %+v", seeded.Unmatched)
	}
}

// TestCreatePrefersTheStoredSettingsOverTheLastSavedDraft pins the precedence:
// once a deployment-wide value exists, it is the one a new record starts from —
// a record whose draft says something else never wins.
func TestCreatePrefersStoredGlobalSettingsOverStaleDraft(t *testing.T) {
	f := newFixture(t)
	_, second := f.twoLibraries(t)
	f.saveSettings(elsewhere("反転"))

	// A record still holding the seed settings, saved after the deployment-wide
	// value was established (an older build, a hand-edited row).
	f.exec(
		"UPDATE workset_operation_drafts SET draft_json = ?, updated_at = ? WHERE workset_id = ?",
		string(draftJSON(t, draftDoc())), stamp(), second.WorksetID,
	)

	fresh := f.createCurrentFor("lib-2", "重建", "albumC")
	seeded := mustDraft(t, f.draft(fresh.WorksetID))
	if seeded.Mode != reconcile.ModeStrict || seeded.DeleteMode != "hard" ||
		len(seeded.NoEffectTags) != 1 || seeded.NoEffectTags[0] != "反転" {
		t.Fatalf("the stored global value must win over a stale draft: %+v", seeded)
	}
}

// TestGlobalSaveBusyErrorNamesTheRefusingRecord pins the typed refusal the
// service turns into the operation-scoped codes.
func TestGlobalSaveBusyErrorNamesTheRefusingRecord(t *testing.T) {
	var refusal *workset.CommonSettingsRefusalError
	raised := error(&workset.CommonSettingsRefusalError{
		WorksetID: "ws-1", Title: "二库", Reason: workset.RefusalExecuting,
	})
	if !errors.As(raised, &refusal) {
		t.Fatal("CommonSettingsRefusalError must be reachable through errors.As")
	}
	if refusal.Title != "二库" || refusal.Error() == "" {
		t.Fatalf("refusal = %+v", refusal)
	}
}

// TestCommonSettingsSaveRefusedOnAnUnreadableDraft covers the other refusal:
// a record whose stored document cannot be read never leaves the save half
// done — every record keeps what it had, and no payload is stored.
func TestCommonSettingsSaveRefusedOnAnUnreadableDraft(t *testing.T) {
	f := newFixture(t)
	first, second := f.twoLibraries(t)
	f.exec(
		"UPDATE workset_operation_drafts SET draft_json = ? WHERE workset_id = ?",
		`{"schema_version":1,"unknown_field":true}`, second.WorksetID,
	)

	before := f.operation(first.WorksetID)
	mineBefore := f.draft(first.WorksetID)
	current, _ := f.svc.GetTaskSettings(f.ctx, workset.OperationTypeConversion)
	_, err := f.trySaveSettings(elsewhere("SEなし"), current.ETag)
	if werr, ok := workset.AsError(err); !ok || werr.Code != "COMMON_SETTINGS_UNREADABLE" {
		t.Fatalf("want COMMON_SETTINGS_UNREADABLE, got %v", err)
	}
	if got := f.operation(first.WorksetID).Version; got != before.Version {
		t.Fatalf("the refused save advanced a record: %d -> %d", before.Version, got)
	}
	if got := f.draft(first.WorksetID); string(got.Document) != string(mineBefore.Document) {
		t.Fatalf("the refused save rewrote a draft: %s", got.Document)
	}
	if common, commonErr := f.repo.GetCommonSettings(workset.OperationTypeConversion); commonErr != nil ||
		common != nil {
		t.Fatalf("a refused save stored settings: %+v err=%v", common, commonErr)
	}
}

// TestDraftSaveRefusesADifferingCommonSection covers the draft PUT's side of
// the rule: it neither applies nor ignores a common section that differs from
// the Task settings, and member-only saves go through.
func TestDraftSaveRefusesADifferingCommonSection(t *testing.T) {
	f := newFixture(t)
	first, _ := f.twoLibraries(t)
	version := first.Operations[0].Version
	differs := func() error {
		_, err := f.svc.SaveDraft(f.ctx, first.WorksetID, workset.OperationTypeConversion,
			workset.SaveDraftRequest{Document: draftJSON(t, elsewhere("反転")), IfMatchVersion: version})
		return err
	}
	// Nothing stored: the record's own draft is the reference.
	if werr, ok := workset.AsError(differs()); !ok || werr.Code != "COMMON_SETTINGS_MISMATCH" {
		t.Fatalf("want COMMON_SETTINGS_MISMATCH without stored settings, got %v", werr)
	}
	f.saveSettings(elsewhere("SEなし"))
	version = f.operation(first.WorksetID).Version
	if werr, ok := workset.AsError(differs()); !ok || werr.Code != "COMMON_SETTINGS_MISMATCH" {
		t.Fatalf("want COMMON_SETTINGS_MISMATCH against stored settings, got %v", werr)
	}
	if common, _ := f.repo.GetCommonSettings(workset.OperationTypeConversion); common == nil ||
		!strings.Contains(common.Settings, "SEなし") {
		t.Fatalf("a refused draft save touched the stored settings: %+v", common)
	}
	if got := f.operation(first.WorksetID).Version; got != version {
		t.Fatalf("the refused save advanced the record: %d -> %d", version, got)
	}
	// A member-only save of the synced common section is accepted.
	doc := mustDraft(t, f.draft(first.WorksetID))
	doc.Members = []conversion.DraftMember{{MemberID: first.Members[0].MemberID, Excluded: true}}
	f.saveDraft(first.WorksetID, doc, version)
}

// Settings stored under an older field spelling read as the same settings: the
// endpoint serves them under today's names and a draft save is not a mismatch,
// while no stored draft is rewritten by either read.
func TestLegacyCommonSettingsReadDoesNotRewriteStoredDraft(t *testing.T) {
	f := newFixture(t)
	first, _ := f.twoLibraries(t)
	f.saveSettings(elsewhere("NoSE"))
	before := f.draft(first.WorksetID)
	legacy := strings.ReplaceAll(string(before.Document), "no_effect_tags", "classifier_tags")
	f.exec("UPDATE workset_operation_drafts SET draft_json = ? WHERE workset_id = ?", legacy, first.WorksetID)
	f.exec(
		"UPDATE common_settings SET settings_json = replace(settings_json, 'no_effect_tags', 'classifier_tags') WHERE operation_type = 'conversion'",
	)
	read, err := f.svc.GetTaskSettings(f.ctx, workset.OperationTypeConversion)
	if err != nil || !strings.Contains(string(read.Settings), "no_effect_tags") {
		t.Fatalf("legacy settings read = %+v err=%v", read, err)
	}
	after := f.draft(first.WorksetID)
	if string(after.Document) != legacy || after.Version != before.Version || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("read changed frozen storage: %+v -> %+v", before, after)
	}
	doc := mustDraft(t, after)
	f.saveDraft(first.WorksetID, doc, after.Version)
}
