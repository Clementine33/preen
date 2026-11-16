package refresh

import "github.com/Clementine33/preen/backend/internal/works"

// Mode selects independently refreshable data groups.
type Mode string

const (
	FillMissing    Mode = "fill_missing"
	RefreshDynamic Mode = "refresh_dynamic"
	RefreshAll     Mode = "refresh_all"
)

// Request names known catalog works, never arbitrary web URLs.
type Request struct {
	WorkCodes []string
	Mode      Mode
	Locale    string
}

// Issue identifies a failure without hiding other works' committed results.
type Issue struct {
	Group   works.DataGroup `json:"group,omitempty"`
	Source  works.Source    `json:"source,omitempty"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
}

// WorkResult contains a work's import outcome and channel failures.
type WorkResult struct {
	WorkCode  works.WorkCode      `json:"rj"`
	MainURL   string              `json:"main_url,omitempty"`
	BackupURL string              `json:"backup_url,omitempty"`
	Outcome   works.ImportOutcome `json:"outcome"`
	Issues    []Issue             `json:"issues"`
}

// nonNilOutcome keeps freshly written Task results free of null arrays; stored
// results from earlier versions may still hold them (docs/api/common.md#wire-schemas).
func nonNilOutcome(o works.ImportOutcome) works.ImportOutcome {
	if o.Applied == nil {
		o.Applied = []works.AppliedGroup{}
	}
	if o.Ignored == nil {
		o.Ignored = []works.IgnoredGroup{}
	}
	if o.Warnings == nil {
		o.Warnings = []works.RelationWarning{}
	}
	return o
}

// Result is ordered by canonical work number, including partial results on cancellation.
type Result struct{ Works []WorkResult }
