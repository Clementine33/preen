package conversion_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"

	"github.com/onsei/organizer/backend/internal/adapters/media"

	"github.com/onsei/organizer/backend/internal/adapters/ffmpeg"
	appconfig "github.com/onsei/organizer/backend/internal/adapters/settings"
	"github.com/onsei/organizer/backend/internal/conversion"
	"github.com/onsei/organizer/backend/internal/conversion/reconcile"
	"github.com/onsei/organizer/backend/internal/workset"
)

func TestConversionRecoversBesideNestedWorkRoot(t *testing.T) {
	library := t.TempDir()
	root := filepath.Join(library, "分类", "RJ000001")
	source := filepath.Join(root, "disc", "obsolete.mp3")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("obsolete"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := json.Marshal(reconcile.ComponentOutcome{
		ComponentID: "delete-only", Status: reconcile.StatusOK,
		Files: []reconcile.FileTuple{{Path: filepath.ToSlash(source), Size: info.Size(), Mtime: info.ModTime().Unix()}},
		Operations: []reconcile.Operation{{
			ComponentID: "delete-only", Kind: reconcile.OpKindRemoveObsolete,
			Phase: reconcile.PhaseRemoveObsoleteAudio, SourcePath: filepath.ToSlash(source),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	task := conversion.New(
		nil,
		appconfig.NewReader(t.TempDir()),
		ffmpeg.New,
		media.ProbeBitrate,
		filesystem.Recover,
		filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{Moves: filesystem.ConversionMoves{}, Files: filesystem.ConversionHandles{}},
	)
	prepared, err := task.PrepareUnit(t.Context(), workset.UnitRunInput{
		WorksetRoot: library, Outcome: outcome, Options: json.RawMessage(`{"delete_mode":"soft"}`),
		Unit: workset.ExecutionUnit{RootPath: filepath.ToSlash(root), Payload: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Commit(t.Context())
	if err != nil || result.ErrorCode != "" || result.InventoryError != "" {
		t.Fatalf("commit = %+v, err = %v", result, err)
	}
	recovered := filepath.Join(library, "分类", "Delete", "RJ000001", "disc", "obsolete.mp3")
	if !slices.Equal(result.Recovery, []string{filepath.ToSlash(recovered)}) {
		t.Fatalf("recovery = %v, want %s", result.Recovery, recovered)
	}
	content, err := os.ReadFile(recovered)
	if err != nil || string(content) != "obsolete" {
		t.Fatalf("recovered bytes = %q, err = %v", content, err)
	}
	if !slices.Equal(result.InventoryRemoved, []string{filepath.ToSlash(source)}) ||
		len(result.InventoryRefreshed) != 0 {
		t.Fatalf("inventory facts include recovery media: %+v", result)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("obsolete source remains: %v", err)
	}
}

// draftJSON builds a minimal structurally valid draft with the given
// obsolete-audio handling (empty omits the field entirely).
func draftJSON(deleteMode string) []byte {
	base := `{"schema_version":1,"no_effect_tags":[],"matched":{},"unmatched":{}}`
	if deleteMode == "" {
		return []byte(base)
	}
	return []byte(strings.TrimSuffix(base, "}") + `,"delete_mode":"` + deleteMode + `"}`)
}

// TestDraftDeleteModeValidation pins the draft's obsolete-audio field: the two
// accepted values, the soft default (an absent field), and a rejection for
// anything else.
func TestDraftDeleteModeValidation(t *testing.T) {
	task := conversion.New(
		nil,
		appconfig.NewReader(t.TempDir()),
		ffmpeg.New,
		media.ProbeBitrate,
		filesystem.Recover,
		filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{Moves: filesystem.ConversionMoves{}, Files: filesystem.ConversionHandles{}},
	)

	for _, accepted := range []string{"", "soft", "hard"} {
		if err := task.ValidateDraft(draftJSON(accepted), nil); err != nil {
			t.Fatalf("delete_mode=%q must validate: %v", accepted, err)
		}
	}
	err := task.ValidateDraft(draftJSON("medium"), nil)
	if werr, ok := workset.AsError(err); !ok || werr.Code != "INVALID_DRAFT" {
		t.Fatalf("err = %v, want INVALID_DRAFT", err)
	}
}

// TestNormalizeDraftKeepsDeleteMode proves the setting survives the sparse
// round trip, so a saved choice reaches every later revision snapshot.
func TestNormalizeDraftKeepsDeleteMode(t *testing.T) {
	task := conversion.New(
		nil,
		appconfig.NewReader(t.TempDir()),
		ffmpeg.New,
		media.ProbeBitrate,
		filesystem.Recover,
		filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{Moves: filesystem.ConversionMoves{}, Files: filesystem.ConversionHandles{}},
	)

	canonical, _, _, err := task.NormalizeDraft(draftJSON("hard"), nil)
	if err != nil {
		t.Fatalf("NormalizeDraft: %v", err)
	}
	doc, err := conversion.ParseDraft(string(canonical))
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}
	if doc.DeleteMode != "hard" {
		t.Fatalf("delete mode = %q, want hard", doc.DeleteMode)
	}
}

// TestSeedDraftReadsTheTagLiterals pins where a new draft's classifier tags come
// from: the configuration's maintained literals, and an empty set — never an
// error and never a built-in default — when that configuration cannot be read.
func TestSeedDraftReadsTheTagLiterals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
		want   []string
	}{
		{"configured", `{"prune":{"no_effect_tags":["SEなし"," 反転 "]}}`, []string{"SEなし", "反転"}},
		{"broken", `{not json`, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(tc.config), 0o644); err != nil {
				t.Fatalf("write config: %v", err)
			}
			raw, _, _ := conversion.New(nil, appconfig.NewReader(dir), ffmpeg.New, media.ProbeBitrate, filesystem.Recover, filesystem.IsResolvedWithinRoot, conversion.ExecutionIO{Moves: filesystem.ConversionMoves{}, Files: filesystem.ConversionHandles{}}).
				SeedDraft(nil)
			var doc struct {
				NoEffectTags []string `json:"no_effect_tags"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("seed draft is not readable: %v", err)
			}
			if len(doc.NoEffectTags) != len(tc.want) {
				t.Fatalf("classifier tags = %v, want %v", doc.NoEffectTags, tc.want)
			}
			for i := range tc.want {
				if doc.NoEffectTags[i] != tc.want[i] {
					t.Fatalf("classifier tags = %v, want %v", doc.NoEffectTags, tc.want)
				}
			}
		})
	}
}

// TestCommonSettingsProjectionAndApply covers the two halves of a
// deployment-wide save: the projection carries the five common values and no
// member record, and applying it moves the base without touching a single
// member record (ADR 0008).
func TestCommonSettingsProjectionAndApply(t *testing.T) {
	task := conversion.New(
		nil,
		appconfig.NewReader(t.TempDir()),
		ffmpeg.New,
		media.ProbeBitrate,
		filesystem.Recover,
		filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{Moves: filesystem.ConversionMoves{}, Files: filesystem.ConversionHandles{}},
	)
	strict := reconcile.ModeStrict
	tags := []string{"自定义"}
	doc := &conversion.DraftDoc{
		SchemaVersion: conversion.DraftSchemaVersion,
		Mode:          reconcile.ModeAvailableSources,
		DeleteMode:    "soft",
		NoEffectTags:  []string{"SEなし"},
		Matched:       reconcile.DesiredProfile{Lossless: &reconcile.AudioOutputSpec{Codec: reconcile.CodecWav}},
		Unmatched:     reconcile.DesiredProfile{Lossless: &reconcile.AudioOutputSpec{Codec: reconcile.CodecWav}},
		Members: []conversion.DraftMember{
			{
				MemberID: "m-1",
				Overrides: &conversion.OverrideSet{
					Mode:         &strict,
					NoEffectTags: &tags,
				},
			},
			{MemberID: "m-2", Excluded: true},
		},
	}
	raw, hash, err := conversion.MarshalDraft(doc)
	if err != nil {
		t.Fatalf("MarshalDraft: %v", err)
	}

	projected, err := task.CommonSettings([]byte(raw))
	if err != nil {
		t.Fatalf("CommonSettings: %v", err)
	}
	var fields map[string]json.RawMessage
	if decodeErr := json.Unmarshal(projected, &fields); decodeErr != nil {
		t.Fatalf("projection is not readable: %v", decodeErr)
	}
	want := []string{"mode", "delete_mode", "no_effect_tags", "other_tags", "matched", "unmatched"}
	if len(fields) != len(want) {
		t.Fatalf("projection fields = %v, want exactly %v", fields, want)
	}
	for _, name := range want {
		if _, ok := fields[name]; !ok {
			t.Fatalf("projection is missing %s: %v", name, fields)
		}
	}
	if _, ok := fields["members"]; ok {
		t.Fatal("a common-settings projection must never carry member records")
	}

	replacement := json.RawMessage(
		`{"mode":"strict","delete_mode":"hard","no_effect_tags":["反転"],` +
			`"matched":{"lossless":{"codec":"flac"}},"unmatched":{"lossless":{"codec":"wav"}}}`,
	)
	updated, updatedHash, err := task.ApplyCommonSettings([]byte(raw), replacement)
	if err != nil {
		t.Fatalf("ApplyCommonSettings: %v", err)
	}
	if updatedHash == hash {
		t.Fatal("a rewritten document must hash differently")
	}
	rewritten, err := conversion.ParseDraft(string(updated))
	if err != nil {
		t.Fatalf("ParseDraft: %v", err)
	}
	if rewritten.Mode != reconcile.ModeStrict || rewritten.DeleteMode != "hard" ||
		len(rewritten.NoEffectTags) != 1 || rewritten.NoEffectTags[0] != "反転" {
		t.Fatalf("common values were not replaced: %+v", rewritten)
	}
	if len(rewritten.Members) != 2 {
		t.Fatalf("member records = %+v, want both", rewritten.Members)
	}
	first := rewritten.Members[0]
	if first.MemberID != "m-1" || first.Overrides == nil || first.Overrides.Mode == nil ||
		*first.Overrides.Mode != reconcile.ModeStrict || first.Overrides.NoEffectTags == nil ||
		(*first.Overrides.NoEffectTags)[0] != "自定义" {
		t.Fatalf("member overrides were not preserved: %+v", first)
	}
	if !rewritten.Members[1].Excluded {
		t.Fatalf("member exclusion was not preserved: %+v", rewritten.Members[1])
	}
	// The hash the rewrite reports is the hash of the document it produced.
	if again, wantHash, hashErr := conversion.MarshalDraft(rewritten); hashErr != nil ||
		again != string(updated) || wantHash != updatedHash {
		t.Fatalf("reported hash does not belong to the document: %s", updated)
	}
}

func TestCommonSettingsRefusesUnknownStoredDraftSchema(t *testing.T) {
	task := conversion.New(
		nil,
		appconfig.NewReader(t.TempDir()),
		ffmpeg.New,
		media.ProbeBitrate,
		filesystem.Recover,
		filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{Moves: filesystem.ConversionMoves{}, Files: filesystem.ConversionHandles{}},
	)
	stored := []byte(`{"schema_version":2,"no_effect_tags":[],"matched":{},"unmatched":{}}`)
	if _, err := task.CommonSettings(stored); err == nil {
		t.Fatal("CommonSettings accepted an unknown stored draft schema")
	}
	if _, _, err := task.ApplyCommonSettings(stored, json.RawMessage(`{}`)); err == nil {
		t.Fatal("ApplyCommonSettings rewrote an unknown stored draft schema")
	}
}

// TestSeedDraftUsesTheStoredCommonSettings pins what a new record starts from:
// the deployment's stored payload when there is one, its own seed values when
// there is none, and the seed values again when the payload cannot be read.
func TestSeedDraftUsesTheStoredCommonSettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "config.json"),
		[]byte(`{"prune":{"no_effect_tags":["SEなし"]}}`),
		0o644,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}
	task := conversion.New(
		nil,
		appconfig.NewReader(dir),
		ffmpeg.New,
		media.ProbeBitrate,
		filesystem.Recover,
		filesystem.IsResolvedWithinRoot,
		conversion.ExecutionIO{Moves: filesystem.ConversionMoves{}, Files: filesystem.ConversionHandles{}},
	)

	stored := json.RawMessage(
		`{"mode":"strict","delete_mode":"hard","no_effect_tags":["反転"],` +
			`"matched":{"lossless":{"codec":"flac"}},"unmatched":{}}`,
	)
	raw, _, _ := task.SeedDraft(stored)
	doc, err := conversion.ParseDraft(string(raw))
	if err != nil {
		t.Fatalf("seed draft is not readable: %v", err)
	}
	if doc.Mode != reconcile.ModeStrict || doc.DeleteMode != "hard" ||
		len(doc.NoEffectTags) != 1 || doc.NoEffectTags[0] != "反転" ||
		doc.Matched.Lossless == nil || doc.Matched.Lossless.Codec != reconcile.CodecFlac {
		t.Fatalf("seed did not take the stored common settings: %+v", doc)
	}
	if len(doc.Members) != 0 {
		t.Fatalf("a seed draft never carries member records: %+v", doc.Members)
	}

	raw, _, _ = task.SeedDraft(nil)
	doc, err = conversion.ParseDraft(string(raw))
	if err != nil {
		t.Fatalf("seed draft is not readable: %v", err)
	}
	if doc.Mode != reconcile.ModeAvailableSources || len(doc.NoEffectTags) != 1 ||
		doc.NoEffectTags[0] != "SEなし" {
		t.Fatalf("seed without stored settings = %+v", doc)
	}

	raw, _, _ = task.SeedDraft(json.RawMessage(`{not json`))
	doc, err = conversion.ParseDraft(string(raw))
	if err != nil {
		t.Fatalf("seed draft is not readable: %v", err)
	}
	if doc.Mode != reconcile.ModeAvailableSources || len(doc.NoEffectTags) != 1 {
		t.Fatalf("an unreadable stored payload must fall back to the seed: %+v", doc)
	}
}
