package metadata

import "github.com/onsei/organizer/backend/internal/works"

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
	Group   works.DataGroup
	Source  works.Source
	Code    string
	Message string
}

// WorkResult contains a work's import outcome and channel failures.
type WorkResult struct {
	WorkCode works.WorkCode
	Outcome  works.ImportOutcome
	Issues   []Issue
}

// Result is ordered by canonical work number, including partial results on cancellation.
type Result struct{ Works []WorkResult }
