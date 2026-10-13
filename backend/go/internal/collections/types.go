package collections

import (
	"math"

	"github.com/onsei/organizer/backend/internal/works"
)

const FavoritesID = "favorites"

const (
	CodeNotFound       = "COLLECTION_NOT_FOUND"
	CodeBuiltin        = "BUILTIN_COLLECTION"
	CodeMemberConflict = "COLLECTION_MEMBER_CONFLICT"
)

type Collection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	WorkCount int    `json:"work_count"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Page struct {
	Collections []Collection `json:"collections"`
	Total       int          `json:"total"`
}

type Detail struct {
	Collection

	Works []works.WorkSummary
	Total int
}

type AppendResult struct {
	Added     []works.WorkCode `json:"added"`
	Skipped   []works.WorkCode `json:"skipped"`
	WorkCount int              `json:"work_count"`
}

// Window uses the catalog's numbered paging conventions.
type Window struct{ Limit, Page int }

func (w Window) Validate() *works.Error {
	if w.Limit < 1 || w.Limit > works.MaxLimit || w.Page < 1 {
		return works.Invalid(works.CodeInvalidPage, "invalid collection page")
	}
	return nil
}

func (w Window) Offset() int {
	if w.Page-1 > math.MaxInt/w.Limit {
		return math.MaxInt
	}
	return (w.Page - 1) * w.Limit
}
