package dirrename

import "context"

// LightPlanning declares directory-only generation without inventory scans or probes.
func (*Task) LightPlanning() {}

// RefreshGenerationInputs bypasses audio inventory refresh: planning reads only
// the selected works' parents for names and target occupancy.
func (*Task) RefreshGenerationInputs(
	ctx context.Context, _ []string, _ string, onRoot func(), _ func() error,
) error {
	if onRoot != nil {
		onRoot()
	}
	return ctx.Err()
}
