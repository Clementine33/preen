package httpapi_test

import "time"

// Response shapes decoded by the handler tests. They mirror the wire JSON
// rather than the package's private DTOs so the tests stay on the public API.

type libraryBody struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	RootPath       string     `json:"root_path"`
	StorageClass   string     `json:"storage_class"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastScanAt     *time.Time `json:"last_scan_at"`
	LastScanStatus string     `json:"last_scan_status"`
	LastScanError  string     `json:"last_scan_error"`
}

type dirBody struct {
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	RelPath        string    `json:"rel_path"`
	DirID          string    `json:"dir_id"`
	AudioFileCount int       `json:"audio_file_count"`
	FileCount      int       `json:"file_count"`
	AddedAt        time.Time `json:"added_at"`
	WorkCode       *string   `json:"work_code"`
}

type memberBody struct {
	MemberID   string `json:"member_id"`
	FolderPath string `json:"folder_path"`
	FolderName string `json:"folder_name"`
	RelPath    string `json:"rel_path"`
	DirID      string `json:"dir_id"`
}

type userTagBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type relationBody struct {
	Role       *string `json:"role"`
	Language   *string `json:"language"`
	OriginalRJ *string `json:"original_rj"`
	ParentRJ   *string `json:"parent_rj"`
}

type treeNodeBody struct {
	RJ        string       `json:"rj"`
	Role      string       `json:"role"`
	IsWork    bool         `json:"is_work"`
	Held      bool         `json:"held"`
	Language  *string      `json:"language"`
	Relation  relationBody `json:"relation"`
	SourceURL *string      `json:"source_url"`
}

type editionSlotBody struct {
	Language     string         `json:"language"`
	Label        *string        `json:"label"`
	Held         bool           `json:"held"`
	Root         treeNodeBody   `json:"root"`
	Translations []treeNodeBody `json:"translations"`
}
