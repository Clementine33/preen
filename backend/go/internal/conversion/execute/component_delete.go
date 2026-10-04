package execute

import (
	"context"
	"fmt"
)

// removeObsolete removes one obsolete file per the selected mode. Soft removal
// reports the recovery location in persisted form; hard removal reports none.
func removeObsolete(
	ctx context.Context,
	tk *componentToolkit,
	workRoot, source string,
	soft bool,
) (string, bool, error) {
	if !soft {
		if err := tk.remove(source); err != nil {
			return "", false, fmt.Errorf("hard delete %s: %w", posixForm(source), err)
		}
		return "", true, nil
	}
	dest, result, err := tk.recoverMove(ctx, workRoot, source)
	if dest != "" {
		dest = posixForm(dest)
	}
	if err == nil && (!result.SourceRemoved || result.Unverified) {
		err = fmt.Errorf("recovery move incomplete")
	}
	return dest, result.SourceRemoved && !result.Unverified, err
}
