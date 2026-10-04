package execute

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// tempOutputPath returns this run's unique staging path beside its target, so
// the commit rename stays on one filesystem and a concurrent run cannot share
// the file.
func tempOutputPath(target string) string {
	return filepath.Join(filepath.Dir(target), filepath.Base(target)+".tmp."+uuid.NewString()[:8])
}

// replacedAsidePath returns the staging name that holds a replaced old output
// until the new output has landed (hard mode only).
func replacedAsidePath(target string) string {
	return filepath.Join(filepath.Dir(target), filepath.Base(target)+".tmp-old."+uuid.NewString()[:8])
}

// commitOutput moves one validated temporary output onto its target. A fresh
// target is renamed into place directly. A same-path replacement first keeps
// the old file recoverable: soft mode moves it under Delete/ before the new
// output lands; hard mode renames it aside until the new output has landed and
// removes the copy afterwards. The boolean reports whether the new output
// landed — a cleanup failure after a landing output still reports true — and
// the returned recovery paths (persisted form) still hold old media whenever
// replacement or cleanup did not fully complete.
func commitOutput(
	ctx context.Context, tk *componentToolkit, workRoot string, enc *plannedEncode, soft bool,
) ([]string, bool, error) {
	target := posixForm(enc.target)
	if !enc.replace {
		return publishOutput(tk, enc)
	}
	var protected string
	var result MoveResult
	var err error
	if soft {
		protected, result, err = tk.recoverMove(ctx, workRoot, enc.target)
	} else {
		protected = replacedAsidePath(enc.target)
		result, err = tk.move(enc.target, protected)
	}
	if err != nil || !result.SourceRemoved || result.Unverified {
		paths := []string{}
		if result.TargetCreated || result.Unverified {
			paths = append(paths, posixForm(protected))
		}
		if result.Unverified {
			paths = append(paths, target)
		}
		return paths, false, fmt.Errorf("protect replaced output %s: %w", target, err)
	}
	paths, committed, publishErr := publishOutput(tk, enc)
	if publishErr != nil {
		// A target known to contain the new output must never be restored over.
		if committed || enc.protected {
			return append(paths, posixForm(protected)), committed, publishErr
		}
		restored, restoreErr := tk.move(protected, enc.target)
		if restoreErr != nil || !restored.SourceRemoved || restored.Unverified {
			return []string{
				posixForm(protected),
				target,
			}, false, fmt.Errorf(
				"commit %s: %w; restore failed: %w",
				target,
				publishErr,
				restoreErr,
			)
		}
		return nil, false, publishErr
	}
	if soft {
		return []string{posixForm(protected)}, true, nil
	}
	if err := tk.remove(protected); err != nil {
		return []string{posixForm(protected)}, true, fmt.Errorf("remove replaced old output: %w", err)
	}
	return nil, true, nil
}

func publishOutput(tk *componentToolkit, enc *plannedEncode) ([]string, bool, error) {
	result, err := tk.move(enc.temp, enc.target)
	if result.Unverified || (result.TargetCreated && !result.SourceRemoved) {
		enc.protected = true
	}
	if result.Unverified {
		return []string{posixForm(enc.target)}, false, fmt.Errorf("publish outcome unverified: %w", err)
	}
	if err != nil {
		return nil, result.TargetCreated, fmt.Errorf("commit %s: %w", posixForm(enc.target), err)
	}
	if !result.TargetCreated || !result.SourceRemoved {
		return nil, result.TargetCreated, fmt.Errorf("incomplete publish")
	}
	enc.temp = ""
	return nil, true, nil
}
