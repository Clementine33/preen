package execute

import (
	"context"
	"fmt"
)

// removeObsolete removes one obsolete file per the selected mode. Soft removal
// reports the recovery location in persisted form; hard removal reports none.
func removeObsolete(ctx context.Context, tk *componentToolkit, workRoot, source string, soft bool) (string, error) {
	if !soft {
		if err := tk.remove(source); err != nil {
			return "", fmt.Errorf("hard delete %s: %w", posixForm(source), err)
		}
		return "", nil
	}
	dest, err := tk.recoverFile(ctx, workRoot, source)
	if err != nil {
		return "", err
	}
	return posixForm(dest), nil
}
