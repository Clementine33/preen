package refresh_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/filesystem"
	"github.com/onsei/organizer/backend/internal/adapters/scraper"
	"github.com/onsei/organizer/backend/internal/adapters/sqlite"
	"github.com/onsei/organizer/backend/internal/inventory"
	"github.com/onsei/organizer/backend/internal/works"
	"github.com/onsei/organizer/backend/internal/works/refresh"
)

// localTransport keeps production source URLs while serving recorded responses locally.
type localTransport struct{ server *httptest.Server }

func (r localTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = r.server.Listener.Addr().String()
	return r.server.Client().Transport.RoundTrip(clone)
}

func catalog(t *testing.T, names ...string) (*sqlite.Repository, *works.Service) {
	t.Helper()
	root := t.TempDir()
	repo, err := sqlite.NewDevelopmentRepository(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	lib, err := repo.CreateLibrary("test", root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		nil,
	)
	if _, err := pipeline.ScanRootCtx(t.Context(), root, inventory.WithLibrary(lib.ID)); err != nil {
		t.Fatal(err)
	}
	return repo, works.NewService(repo, nil, nil, nil, "")
}

func TestRefreshImportsRequestedWorkWithoutChangingPersonalData(t *testing.T) {
	repo, worksSvc := catalog(t, "RJ000001 old")
	id := works.WorkCode("RJ00000001")
	note := "my note"
	if _, err := worksSvc.PatchUserData(id, works.UserDataRequest{Note: &note}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/maniax/api/=/product.json":
			if r.URL.Query().Get("workno") != "RJ000001" || r.URL.Query().Get("locale") != "ja-jp" {
				t.Errorf("unexpected request: %s", r.URL)
			}
			_, _ = w.Write(
				[]byte(
					`[{"workno":"RJ000001","product_name":"作品 / タイトル","maker_id":"RG1","maker_name":"circle","age_category":3,"regist_date":"2025-03-01 00:00:00","genres":[],"creaters":{"voice_by":[]},"language_editions":[]}]`,
				),
			)
		case "/maniax-touch/product/info/ajax":
			_, _ = w.Write(
				[]byte(
					`{"RJ000001":{"dl_count":42,"translation_info":{"is_original":true,"is_parent":false,"is_child":false,"original_workno":null,"parent_workno":null,"child_worknos":[],"lang":null}}}`,
				),
			)
		default:
			t.Errorf("unexpected fallback: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := scraper.New(&http.Client{Transport: localTransport{server}})
	svc := refresh.NewService(worksSvc, repo, client, client)
	result, err := svc.Refresh(
		context.Background(),
		refresh.Request{WorkCodes: []string{"RJ000001", "RJ00000001"}, Mode: refresh.RefreshAll},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Works) != 1 || len(result.Works[0].Outcome.Applied) != 4 || len(result.Works[0].Issues) != 0 {
		t.Fatalf("refresh: %+v", result)
	}
	detail, detailErr := worksSvc.Detail(id)
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	if detail.Title == nil || *detail.Title != "作品 / タイトル" || detail.DLCount == nil || *detail.DLCount != 42 ||
		detail.Note != note ||
		len(detail.Copies) != 1 {
		t.Fatalf("detail: %+v", detail)
	}
}

func TestDynamicRefreshLeavesCoreAndEditionsUntouched(t *testing.T) {
	repo, worksSvc := catalog(t, "RJ000001 old", "RJ000002 old")
	title := "saved title"
	first := works.WorkCode("RJ00000001")
	if _, err := worksSvc.Import(
		first,
		works.ImportRequest{
			Core: &works.ImportCore{Source: "dlsite", FetchedAt: "2020-01-01T00:00:00.000000000Z", Title: &title},
			Editions: &works.ImportEditions{
				Source:    "dlsite",
				FetchedAt: "2020-01-01T00:00:00.000000000Z",
				Items:     []works.ImportEditionItem{{VersionRootRJ: "RJ00000001", Language: "JPN"}},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/maniax-touch/product/info/ajax" || r.URL.Query().Get("product_id") != "RJ000001,RJ000002" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		_, _ = w.Write(
			[]byte(
				`{"RJ000001":{"dl_count":55,"translation_info":{"is_original":true}},"RJ000002":{"dl_count":0,"translation_info":{"is_original":true}}}`,
			),
		)
	}))
	t.Cleanup(server.Close)
	client := scraper.New(&http.Client{Transport: localTransport{server}})
	result, err := refresh.NewService(worksSvc, repo, client, client).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{"RJ000002", "RJ000001"}, Mode: refresh.RefreshDynamic})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Works) != 2 || result.Works[0].WorkCode != first {
		t.Fatalf("result: %+v", result)
	}
	detail, detailErr := worksSvc.Detail(first)
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	if detail.Title == nil || *detail.Title != title || len(detail.Editions) != 1 || detail.DLCount == nil ||
		*detail.DLCount != 55 {
		t.Fatalf("detail: %+v", detail)
	}
}

func TestMissingOnlyRefreshDoesNotFetchAnAlreadyCompleteWork(t *testing.T) {
	repo, worksSvc := catalog(t, "RJ000001 old")
	title := "existing"
	_, err := worksSvc.Import(works.WorkCode("RJ00000001"), works.ImportRequest{
		Core:       &works.ImportCore{Source: "dlsite", FetchedAt: "2020-01-01T00:00:00.000000000Z", Title: &title},
		Statistics: &works.ImportStatistics{FetchedAt: "2020-01-01T00:00:00.000000000Z"},
		Relations: &works.ImportRelations{
			Source:    "dlsite",
			FetchedAt: "2020-01-01T00:00:00.000000000Z",
			Role:      "original",
		},
		Editions: &works.ImportEditions{Source: "dlsite", FetchedAt: "2020-01-01T00:00:00.000000000Z"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("complete work was fetched: %s", r.URL)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	client := scraper.New(&http.Client{Transport: localTransport{server}})
	result, refreshErr := refresh.NewService(worksSvc, repo, client, client).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{"RJ000001"}})
	if refreshErr != nil || len(result.Works) != 1 || len(result.Works[0].Outcome.Applied) != 0 ||
		len(result.Works[0].Issues) != 0 {
		t.Fatalf("refresh: %+v, %v", result, refreshErr)
	}
}

func TestFallbackOnlySuppliesUnavailableGroups(t *testing.T) {
	repo, worksSvc := catalog(t, "RJ000001 old")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/maniax/api/=/product.json":
			_, _ = w.Write([]byte(`[]`))
		case "/maniax-touch/product/info/ajax":
			_, _ = w.Write([]byte(`{"RJ000001":{"dl_count":77,"translation_info":{"is_original":true}}}`))
		case "/api/workInfo/1":
			_, _ = w.Write(
				[]byte(
					`{"source_id":"RJ000001","title":"backup title","circle":{"id":7,"name":"backup circle"},"vas":[],"tags":[],"dl_count":999,"language_editions":[]}`,
				),
			)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := scraper.New(&http.Client{Transport: localTransport{server}})
	result, err := refresh.NewService(worksSvc, repo, client, client).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{"RJ000001"}, Mode: refresh.RefreshAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Works[0].Outcome.Applied) != 4 {
		t.Fatalf("result: %+v", result)
	}
	detail, detailErr := worksSvc.Detail(works.WorkCode("RJ00000001"))
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	if detail.Title == nil || *detail.Title != "backup title" || detail.DLCount == nil || *detail.DLCount != 77 {
		t.Fatalf("detail: %+v", detail)
	}
	states, stateErr := repo.MetadataStates(t.Context(), []works.WorkCode{"RJ00000001"})
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	if states["RJ00000001"][works.GroupCore].Source != works.SourceASMRone ||
		states["RJ00000001"][works.GroupRelations].Source != works.SourceDLsite {
		t.Fatalf("states: %+v", states)
	}
}

func TestMalformedCoreDoesNotEraseStoredCredits(t *testing.T) {
	repo, worksSvc := catalog(t, "RJ000001 old")
	title := "stored title"
	if _, err := worksSvc.Import(
		"RJ00000001",
		works.ImportRequest{
			Core: &works.ImportCore{
				Source:      "dlsite",
				FetchedAt:   "2020-01-01T00:00:00.000000000Z",
				Title:       &title,
				VoiceActors: []works.ImportVoiceActor{{Name: "saved actor"}},
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/maniax/api/=/product.json":
			_, _ = w.Write(
				[]byte(
					`[{"workno":"RJ000001","product_name":"bad core","creaters":{"voice_by":"broken"},"genres":[],"language_editions":[]}]`,
				),
			)
		case "/maniax-touch/product/info/ajax":
			_, _ = w.Write([]byte(`{"RJ000001":{"dl_count":2,"translation_info":{"is_original":true}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := scraper.New(&http.Client{Transport: localTransport{server}})
	result, err := refresh.NewService(worksSvc, repo, client, client).
		Refresh(t.Context(), refresh.Request{WorkCodes: []string{"RJ000001"}, Mode: refresh.RefreshAll})
	if err != nil {
		t.Fatal(err)
	}
	detail, detailErr := worksSvc.Detail("RJ00000001")
	if detailErr != nil {
		t.Fatal(detailErr)
	}
	if detail.Title == nil || *detail.Title != title || len(detail.VoiceActors) != 1 ||
		detail.VoiceActors[0].Name != "saved actor" ||
		detail.DLCount == nil ||
		*detail.DLCount != 2 ||
		len(result.Works[0].Issues) == 0 {
		t.Fatalf("detail: %+v, result: %+v", detail, result)
	}
}

// waitingSource holds static requests until cancellation to expose the worker bound.
type waitingSource struct {
	calls atomic.Int32
	ready chan struct{}
}

func (s *waitingSource) FetchStatic(ctx context.Context, _ works.WorkCode, _ string) (refresh.Snapshot, error) {
	if s.calls.Add(1) == 8 {
		close(s.ready)
	}
	<-ctx.Done()
	return refresh.Snapshot{}, ctx.Err()
}

func (*waitingSource) FetchDynamic(
	context.Context,
	[]works.WorkCode,
	string,
) (map[works.WorkCode]works.ImportRequest, error) {
	return map[works.WorkCode]works.ImportRequest{}, nil
}
func (*waitingSource) FetchASMR(ctx context.Context, _ works.WorkCode, _ string) (refresh.Snapshot, error) {
	return refresh.Snapshot{}, ctx.Err()
}

func TestStaticWorkersAreBoundedAndCancellationDoesNotImport(t *testing.T) {
	names := []string{}
	codes := []string{}
	for n := 1; n <= 12; n++ {
		names = append(names, fmt.Sprintf("RJ%06d", n))
		codes = append(codes, fmt.Sprintf("RJ%06d", n))
	}
	repo, catalogSvc := catalog(t, names...)
	source := &waitingSource{ready: make(chan struct{})}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	go func() {
		select {
		case <-source.ready:
			cancel()
		case <-ctx.Done():
		}
	}()
	_, err := refresh.NewService(catalogSvc, repo, source, source).
		Refresh(ctx, refresh.Request{WorkCodes: codes, Mode: refresh.RefreshAll})
	if err == nil || source.calls.Load() != 8 {
		t.Fatalf("static worker bound/cancellation: calls=%d, err=%v", source.calls.Load(), err)
	}
	states, err := repo.MetadataStates(t.Context(), []works.WorkCode{"RJ00000001"})
	if err != nil || len(states["RJ00000001"]) != 0 {
		t.Fatalf("cancelled refresh imported: %v, %v", states, err)
	}
}

func TestProgressObservesCommittedMetadataAndCanStopTheRefresh(t *testing.T) {
	repo, cat := catalog(t, "RJ000001")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/maniax/api/=/product.json" {
			_, _ = w.Write(
				[]byte(
					`[{"workno":"RJ000001","product_name":"fresh","genres":[],"creaters":{},"language_editions":[],"image_main":{"url":"//img.dlsite.jp/main.webp"}}]`,
				),
			)
			return
		}
		_, _ = w.Write([]byte(`{"RJ000001":{"dl_count":1,"translation_info":{"is_original":true}}}`))
	}))
	defer server.Close()
	source := scraper.New(&http.Client{Transport: localTransport{server}})
	seen := false
	_, err := refresh.NewService(cat, repo, source, source).
		RefreshWithProgress(t.Context(), refresh.Request{WorkCodes: []string{"RJ000001"}}, func(item refresh.WorkResult) error {
			detail, e := cat.Detail(item.WorkCode)
			if e != nil || detail.Title == nil || *detail.Title != "fresh" {
				t.Fatal("progress before import commit")
			}
			if item.MainURL != "https://img.dlsite.jp/main.webp" {
				t.Fatalf("source response not reused: %+v", item)
			}
			seen = true
			return fmt.Errorf("stop after durable progress")
		})
	if !seen || err == nil {
		t.Fatalf("callback stop ignored: %v %v", seen, err)
	}
}
