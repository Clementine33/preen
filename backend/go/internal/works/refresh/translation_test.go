package refresh_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/onsei/organizer/backend/internal/adapters/scraper"
	"github.com/onsei/organizer/backend/internal/works"
	"github.com/onsei/organizer/backend/internal/works/refresh"
)

//nolint:gocognit // One end-to-end assertion covers title, circle, locale and persisted ancestry together.
func TestChineseTranslationRefreshFollowsAncestorsAndSeparatesTagLocale(t *testing.T) {
	for _, scenario := range []struct{ language, locale, title string }{
		{"CHI_HANS", "ja-jp", "简体中文版标题"},
		{"CHI_HANS", "zh-cn", "简体中文版标题"},
		{"CHI_HANS", "zh-tw", "简体中文版标题"},
		{"CHI_HANT", "ja-jp", "繁體中文版標題"},
		{"CHI_HANT", "zh-cn", "繁體中文版標題"},
		{"CHI_HANT", "zh-tw", "繁體中文版標題"},
	} {
		t.Run(scenario.language+"/"+scenario.locale, func(t *testing.T) {
			locale := scenario.locale
			repo, cat := catalog(t, "RJ01449056")
			server := translationServer(t, false, scenario.language)
			client := scraper.New(&http.Client{Transport: localTransport{server}})
			result, err := refresh.NewService(cat, repo, client, client).Refresh(t.Context(), refresh.Request{
				WorkCodes: []string{"RJ01449056"}, Mode: refresh.RefreshAll, Locale: locale,
			})
			if err != nil || len(result.Works) != 1 || len(result.Works[0].Issues) != 0 {
				t.Fatalf("refresh: %+v, %v", result, err)
			}
			for _, id := range []works.WorkCode{"RJ01449056", "RJ01449055", "RJ01407907"} {
				detail, e := cat.Detail(id)
				if e != nil || detail.Title == nil {
					t.Fatalf("missing ancestor metadata for %s: %+v, %v", id, detail, e)
				}
				if detail.CircleName == nil || *detail.CircleName != "桃色CODE" {
					t.Fatalf("%s has translator/platform circle: %+v", id, detail.CircleName)
				}
				if id != "RJ01407907" && *detail.Title != scenario.title {
					t.Fatalf("%s has wrong title: %s", id, *detail.Title)
				}
				if len(detail.SourceTags) != 1 || detail.SourceTags[0].Name != locale {
					t.Fatalf("%s has wrong tag locale: %+v", id, detail.SourceTags)
				}
				if len(detail.Editions) != 1 || detail.Editions[0].Language != scenario.language {
					t.Fatalf("unfiltered editions: %+v", detail.Editions)
				}
				if id != "RJ01449056" && !detail.Placeholder {
					t.Fatalf("ancestor %s became collected", id)
				}
			}
			if _, e := cat.Detail("RJ09999999"); e == nil {
				t.Fatal("English edition created a placeholder")
			}
			child, _ := cat.Detail("RJ01449056")
			if child.Relation.ParentID == nil || *child.Relation.ParentID != "RJ01449055" ||
				child.Relation.OriginalID == nil || *child.Relation.OriginalID != "RJ01407907" {
				t.Fatalf("incomplete relation: %+v", child.Relation)
			}
		})
	}
}

func TestUnavailableOriginalPreservesTranslationCore(t *testing.T) {
	repo, cat := catalog(t, "RJ01449056")
	title := "saved title"
	if _, err := cat.Import("RJ01449056", works.ImportRequest{Core: &works.ImportCore{
		Source: "dlsite", FetchedAt: "2020-01-01T00:00:00.000000000Z", Title: &title,
		Circle: &works.ImportCircle{Name: "saved circle"},
	}}); err != nil {
		t.Fatal(err)
	}
	server := translationServer(t, true, "CHI_HANS")
	client := scraper.New(&http.Client{Transport: localTransport{server}})
	result, err := refresh.NewService(cat, repo, client, client).Refresh(t.Context(), refresh.Request{
		WorkCodes: []string{"RJ01449056"}, Mode: refresh.RefreshAll,
	})
	if err != nil || len(result.Works[0].Issues) == 0 {
		t.Fatalf("unreported failure: %+v, %v", result, err)
	}
	detail, e := cat.Detail("RJ01449056")
	if e != nil || detail.Title == nil || *detail.Title != title || detail.CircleName == nil ||
		*detail.CircleName != "saved circle" || detail.DLCount == nil || *detail.DLCount != 12 {
		t.Fatalf("failed original overwrote core or lost sales: %+v, %v", detail, e)
	}
}

func TestSelectedTranslationsShareOriginalRequestsAndDoNotCrawlSiblings(t *testing.T) {
	for _, names := range [][]string{{"RJ01449056"}, {"RJ01449056", "RJ01449057"}} {
		t.Run(strings.Join(names, ","), func(t *testing.T) {
			repo, cat := catalog(t, names...)
			server := translationServer(t, false, "CHI_HANS")
			transport := &countingTransport{local: localTransport{server}, calls: map[string]int{}}
			client := scraper.New(&http.Client{Transport: transport})
			result, err := refresh.NewService(cat, repo, client, client).Refresh(t.Context(), refresh.Request{
				WorkCodes: names, Mode: refresh.RefreshAll,
			})
			if err != nil || len(result.Works) != len(names) {
				t.Fatalf("refresh: %+v, %v", result, err)
			}
			if transport.calls["RJ01407907:ja-jp"] != 1 {
				t.Fatalf("original snapshot was fetched repeatedly: %v", transport.calls)
			}
			if len(names) == 1 && transport.calls["RJ01449057:ja-jp"] != 0 {
				t.Fatal("unselected sibling was fetched")
			}
		})
	}
}

type countingTransport struct {
	local localTransport
	mu    sync.Mutex
	calls map[string]int
}

func (r *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if id := req.URL.Query().Get("workno"); id != "" {
		r.mu.Lock()
		r.calls[id+":"+req.URL.Query().Get("locale")]++
		r.mu.Unlock()
	}
	return r.local.RoundTrip(req)
}

func translationServer(t *testing.T, failOriginal bool, language string) *httptest.Server {
	t.Helper()
	reports := map[string]map[string]any{
		"RJ01407907": {"is_original": true},
		"RJ01449055": {"is_parent": true, "original_workno": "RJ01407907", "lang": language,
			"child_worknos": []string{"RJ01449056", "RJ01449057"}},
		"RJ01449056": {"is_child": true, "original_workno": "RJ01407907", "parent_workno": "RJ01449055",
			"lang": language},
		"RJ01449057": {"is_child": true, "original_workno": "RJ01407907", "parent_workno": "RJ01449055",
			"lang": language},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/maniax-touch/product/info/ajax" {
			out := map[string]any{}
			for id := range strings.SplitSeq(r.URL.Query().Get("product_id"), ",") {
				if reports[id] == nil {
					t.Errorf("unrequested sibling fetched: %s", id)
				}
				out[id] = map[string]any{"translation_info": reports[id], "dl_count": 12}
			}
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		id := r.URL.Query().Get("workno")
		if r.URL.Path != "/maniax/api/=/product.json" || reports[id] == nil || failOriginal && id == "RJ01407907" {
			http.NotFound(w, r)
			return
		}
		locale := r.URL.Query().Get("locale")
		title, circle := "日本語タイトル", "translator"
		if id != "RJ01407907" {
			if language == "CHI_HANS" && locale == "zh-cn" {
				title = "简体中文版标题"
			}
			if language == "CHI_HANT" && locale == "zh-tw" {
				title = "繁體中文版標題"
			}
		}
		if id == "RJ01407907" {
			circle = "桃色CODE"
			if locale != "ja-jp" {
				circle = "localized original circle"
			}
		}
		_ = json.NewEncoder(w).Encode([]any{map[string]any{
			"workno": id, "product_name": title, "maker_id": "RG" + id, "maker_name": circle,
			"genres": []any{map[string]any{"id": 68, "name": locale}}, "creaters": map[string]any{},
			"translation_info": reports[id], "language_editions": []any{
				map[string]any{"workno": "RJ01407907", "lang": "JPN"},
				map[string]any{"workno": "RJ01449055", "lang": language},
				map[string]any{"workno": "RJ09999999", "lang": "ENG"},
			},
		}})
	}))
	t.Cleanup(server.Close)
	return server
}
