package scraper

import "github.com/onsei/organizer/backend/internal/works"

// Incomplete groups stay absent, so another source can supply them independently.
func validSnapshot(id works.WorkCode, data works.ImportRequest) works.ImportRequest {
	if data.Core != nil {
		if _, err := works.ValidateImport(id, works.ImportRequest{Core: data.Core}); err != nil {
			data.Core = nil
		}
	}
	if data.Statistics != nil {
		if _, err := works.ValidateImport(id, works.ImportRequest{Statistics: data.Statistics}); err != nil {
			data.Statistics = nil
		}
	}
	if data.Relations != nil {
		if _, err := works.ValidateImport(id, works.ImportRequest{Relations: data.Relations}); err != nil {
			data.Relations = nil
		}
	}
	if data.Editions != nil {
		if _, err := works.ValidateImport(id, works.ImportRequest{Editions: data.Editions}); err != nil {
			data.Editions = nil
		}
	}
	return data
}
