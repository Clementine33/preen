package refresh

import (
	"context"
	"fmt"
	"sync"

	"github.com/Clementine33/preen/backend/internal/works"
)

// refreshRun shares source responses and upstream imports within one refresh.
type refreshRun struct {
	service   *Service
	snapshots sync.Map
	dynamic   map[works.WorkCode]works.ImportRequest
	upstream  sync.Mutex
	finished  map[works.WorkCode][]Issue
}

type snapshotKey struct {
	id     works.WorkCode
	source works.Source
	locale string
}

type snapshotFetch struct {
	ready chan struct{}
	data  Snapshot
	err   error
}

func (r *refreshRun) fetch(
	ctx context.Context,
	id works.WorkCode,
	source works.Source,
	locale string,
) (Snapshot, error) {
	key := snapshotKey{id: id, source: source, locale: locale}
	call := &snapshotFetch{ready: make(chan struct{})}
	actual, loaded := r.snapshots.LoadOrStore(key, call)
	if loaded {
		call = actual.(*snapshotFetch) //nolint:errcheck // Only snapshotFetch values are stored in this map.
	} else {
		if source == works.SourceDLsite {
			call.data, call.err = r.service.dlsite.FetchStatic(ctx, id, locale)
		} else {
			call.data, call.err = r.service.backup.FetchASMR(ctx, id, locale)
		}
		close(call.ready)
	}
	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	case <-call.ready:
		data := call.data
		if data.Core != nil {
			core := *data.Core
			data.Core = &core
		}
		return data, call.err
	}
}

// localized keeps the work's title independent of the requested tag language.
func (r *refreshRun) localized(
	ctx context.Context, id works.WorkCode, source works.Source, tagLocale string, report *works.ImportRelations,
) (Snapshot, error) {
	data, err := r.fetch(ctx, id, source, tagLocale)
	if err != nil {
		return data, err
	}
	if report == nil {
		report = data.Relations
	}
	locale := tagLocale
	if report != nil {
		switch {
		case report.Role == "original":
			locale = "ja-jp"
		case report.Language != nil && *report.Language == "CHI_HANS":
			locale = "zh-cn"
		case report.Language != nil && *report.Language == "CHI_HANT":
			locale = "zh-tw"
		}
	}
	if locale != tagLocale {
		localized, fetchErr := r.fetch(ctx, id, source, locale)
		if fetchErr != nil {
			data.Core = nil
			return data, fetchErr
		}
		if localized.Core != nil && data.Core != nil {
			localized.Core.Tags = data.Core.Tags
		} else {
			localized.Core = nil
		}
		data = localized
	}
	if data.Core != nil && (data.Core.Title == nil || *data.Core.Title == "") {
		data.Core = nil
		return data, fmt.Errorf("%s has no title for %s", id, locale)
	}
	return data, nil
}

func (r *refreshRun) originalCircle(ctx context.Context, data works.ImportRequest) error {
	if data.Core == nil || data.Relations == nil || data.Relations.Role == "original" {
		return nil
	}
	report := data.Relations
	seen := map[works.WorkCode]bool{}
	for {
		raw := report.OriginalRJ
		if raw == nil {
			raw = report.ParentRJ
		}
		if raw == nil {
			return fmt.Errorf("translation has no reported upstream work")
		}
		id, ok := works.ParseWorkCode(*raw)
		if !ok || seen[id] {
			return fmt.Errorf("invalid or cyclic original reference %s", *raw)
		}
		seen[id] = true
		original, err := r.fetch(ctx, id, works.Source(data.Core.Source), "ja-jp")
		if err != nil {
			return fmt.Errorf("original %s: %w", id, err)
		}
		if original.Relations == nil || report.OriginalRJ != nil && original.Relations.Role != "original" {
			return fmt.Errorf("original %s has no confirmed original report", id)
		}
		if original.Relations.Role == "original" {
			if original.Core == nil || original.Core.Circle == nil {
				return fmt.Errorf("original %s has no confirmed original circle", id)
			}
			data.Core.Circle = original.Core.Circle
			return nil
		}
		report = original.Relations
	}
}

func chineseLanguage(language *string) bool {
	return language != nil && (*language == "CHI_HANS" || *language == "CHI_HANT")
}

// chineseSnapshot filters declarations before they can create placeholders.
func chineseSnapshot(data works.ImportRequest) works.ImportRequest {
	if data.Relations != nil && data.Relations.Role != "original" && !chineseLanguage(data.Relations.Language) {
		data.Relations = nil
	}
	if data.Editions != nil {
		editions := *data.Editions
		editions.Items = []works.ImportEditionItem{}
		for _, item := range data.Editions.Items {
			if chineseLanguage(&item.Language) {
				editions.Items = append(editions.Items, item)
			}
		}
		data.Editions = &editions
	}
	if data.Relations != nil && data.Relations.Role == "original" {
		// Only known Chinese edition roots may be declared by the original.
		report := *data.Relations
		report.ChildRJs = []string{}
		chinese := map[string]bool{}
		if data.Editions != nil {
			for _, edition := range data.Editions.Items {
				chinese[edition.VersionRootRJ] = true
			}
		}
		for _, child := range data.Relations.ChildRJs {
			if chinese[child] {
				report.ChildRJs = append(report.ChildRJs, child)
			}
		}
		data.Relations = &report
	}
	return data
}
