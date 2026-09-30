package metadatatask

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/onsei/organizer/backend/internal/covers"
	"github.com/onsei/organizer/backend/internal/metadata"
	"github.com/onsei/organizer/backend/internal/workset"
)

// Executor connects the metadata and delivery entries without coupling generic worksets to either.
type Executor struct {
	Metadata *metadata.Service
	Covers   *covers.Service
}

func (e Executor) Run(ctx context.Context, in workset.BatchExecutionInput) error {
	var common Common
	if err := json.Unmarshal(in.Options, &common); err != nil {
		return err
	}
	if err := validateCommon(common); err != nil {
		return err
	}
	byCode := map[string][]workset.ExecutionUnit{}
	targets := map[string][]covers.Target{}
	codes := []string{}
	for _, unit := range in.Units {
		var o Outcome
		if err := json.Unmarshal(unit.Payload, &o); err != nil {
			return err
		}
		if o.Action == "skip" {
			raw, _ := json.Marshal(o)
			zero := 0
			if err := in.Progress(
				unit.Index,
				workset.UnitResult{TaskResult: raw, CompletedOperations: &zero, Stage: "skipped"},
			); err != nil {
				return err
			}
			continue
		}
		if o.Action != "refresh" || o.Target.CopyID == "" || o.Target.WorkCode == "" {
			return fmt.Errorf("invalid frozen metadata unit")
		}
		if _, ok := byCode[o.Target.WorkCode]; !ok {
			codes = append(codes, o.Target.WorkCode)
		}
		byCode[o.Target.WorkCode] = append(byCode[o.Target.WorkCode], unit)
		targets[o.Target.WorkCode] = append(targets[o.Target.WorkCode], o.Target)
	}
	if len(codes) == 0 {
		return nil
	}
	_, err := e.Metadata.RefreshWithProgress(
		ctx,
		metadata.Request{WorkCodes: codes, Mode: common.Mode, Locale: common.Locale},
		func(item metadata.WorkResult) error {
			code := string(item.WorkCode)
			var queueErr error
			if common.DownloadCover && common.Mode != metadata.RefreshDynamic {
				if e.Covers == nil {
					return fmt.Errorf("cover delivery is not wired")
				}
				queueErr = e.Covers.Enqueue(
					ctx,
					covers.Request{
						ExecutionID:   in.ExecutionID,
						WorkCode:      code,
						URL:           item.MainURL,
						MainChecked:   item.MainChecked,
						BackupChecked: item.BackupChecked,
						BackupURL:     item.BackupURL,
						Locale:        common.Locale,
						Refresh:       common.Mode == metadata.RefreshAll,
						Targets:       targets[code],
					},
				)
			}
			raw, _ := json.Marshal(item)
			for _, unit := range byCode[code] {
				done := unit.Operations
				res := workset.UnitResult{Stage: "metadata", TaskResult: raw, CompletedOperations: &done}
				for _, issue := range item.Issues {
					if issue.Code == "GROUP_UNAVAILABLE" || issue.Source == "" && issue.Code != "" {
						res.ErrorCode = "METADATA_UNAVAILABLE"
						res.ErrorMessage = issue.Message
						done = 0
					}
				}
				if queueErr != nil {
					res.ErrorCode = "COVER_QUEUE_FAILED"
					res.ErrorMessage = queueErr.Error()
				}
				if err := in.Progress(unit.Index, res); err != nil {
					return err
				}
			}
			return queueErr
		},
	)
	return err
}
