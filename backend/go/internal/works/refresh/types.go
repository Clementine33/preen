package refresh

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
	Group   works.DataGroup `json:"group,omitempty"`
	Source  works.Source    `json:"source,omitempty"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
}

// WorkResult contains a work's import outcome and channel failures.
type WorkResult struct {
	MainChecked   bool                `json:"-"`
	BackupChecked bool                `json:"-"`
	WorkCode      works.WorkCode      `json:"rj"`
	MainURL       string              `json:"main_url,omitempty"`
	BackupURL     string              `json:"backup_url,omitempty"`
	Outcome       works.ImportOutcome `json:"outcome"`
	Issues        []Issue             `json:"issues"`
}

// Result is ordered by canonical work number, including partial results on cancellation.
type Result struct{ Works []WorkResult }
