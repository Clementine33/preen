package scraper_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/onsei/organizer/backend/internal/adapters/scraper"
	"github.com/onsei/organizer/backend/internal/works"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestSourceRetriesAreBoundedAndChallengesStopFurtherRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return response(http.StatusServiceUnavailable, `{}`), nil
		})})
		if _, err := client.FetchDynamic(context.Background(), []works.WorkCode{"RJ00000001"}, "ja-jp"); err == nil {
			t.Fatal("503 was accepted")
		}
		if calls.Load() != 3 {
			t.Fatalf("retry cap: got %d requests, want 3", calls.Load())
		}
		calls.Store(0)
		client = scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			r := response(http.StatusTooManyRequests, `{}`)
			r.Header.Set("Retry-After", "120")
			return r, nil
		})})
		_, _ = client.FetchDynamic(context.Background(), []works.WorkCode{"RJ00000001"}, "ja-jp")
		_, _ = client.FetchStatic(context.Background(), "RJ00000002", "ja-jp")
		if calls.Load() != 1 {
			t.Fatalf("cooling source was contacted %d times", calls.Load())
		}
		time.Sleep(61 * time.Second)
		_, _ = client.FetchStatic(context.Background(), "RJ00000002", "ja-jp")
		if calls.Load() != 1 {
			t.Fatal("Retry-After was ignored")
		}
		time.Sleep(60 * time.Second)
		_, _ = client.FetchStatic(context.Background(), "RJ00000002", "ja-jp")
		if calls.Load() != 2 {
			t.Fatal("source did not recover after cooldown")
		}
	})
}

func TestSourcesValidateIdentityAndBackupLocale(t *testing.T) {
	client := scraper.New(&http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.asmr.one" {
			if req.URL.Path != "/api/workInfo/1" {
				t.Errorf("backup path: %s", req.URL.Path)
			}
			cookie, err := req.Cookie("locale")
			if err != nil || cookie.Value != "zh-cn" {
				t.Errorf("locale cookie missing: %v, %v", cookie, err)
			}
			return response(
				200,
				`{"source_id":"RJ000001","title":"backup","vas":"broken","tags":[],"language_editions":[]}`,
			), nil
		}
		return response(
			200,
			`[{"workno":"RJ000002","product_name":"wrong work","genres":[],"creaters":{},"language_editions":[]}]`,
		), nil
	})})
	if _, err := client.FetchStatic(t.Context(), "RJ00000001", "zh-cn"); err == nil {
		t.Fatal("wrong work response accepted")
	}
	backup, err := client.FetchASMR(t.Context(), "RJ00000001", "zh-cn")
	if err != nil {
		t.Fatal(err)
	}
	if backup.Core != nil || backup.Editions == nil {
		t.Fatalf("malformed backup cleared credits or discarded valid editions: %+v", backup)
	}
}

func TestDynamicSplitsOversizedRequestsAndAcceptsNumericStrings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		client := scraper.New(&http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			ids := strings.Split(req.URL.Query().Get("product_id"), ",")
			if len(ids) > 1 {
				return response(414, `{}`), nil
			}
			b, err := json.Marshal(
				map[string]any{
					ids[0]: map[string]any{"dl_count": "42", "translation_info": map[string]bool{"is_original": true}},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			return response(200, string(b)), nil
		})})
		result, err := client.FetchDynamic(t.Context(), []works.WorkCode{"RJ00000001", "RJ00000002"}, "ja-jp")
		if err != nil {
			t.Fatal(err)
		}
		if calls != 3 || len(result) != 2 || *result["RJ00000001"].Statistics.DLCount != 42 {
			t.Fatalf("split result: %v, requests: %d", result, calls)
		}
	})
}

func TestMalformedCreditsNeverBecomeAnEmptySnapshot(t *testing.T) {
	for _, body := range []string{
		`[{"workno":"RJ000001","product_name":"title","genres":[],"creaters":{"voice_by":[{"id":1,"name":""}]},"language_editions":[]}]`,
		`[{"workno":"RJ000001","product_name":"title","genres":[],"creaters":{"voice_by":[{"id":{},"name":"voice"}]},"language_editions":[]}]`,
	} {
		t.Run(body, func(t *testing.T) {
			client := scraper.New(
				&http.Client{
					Transport: transportFunc(
						func(*http.Request) (*http.Response, error) { return response(200, body), nil },
					),
				},
			)
			result, err := client.FetchStatic(t.Context(), "RJ00000001", "ja-jp")
			if err != nil {
				t.Fatal(err)
			}
			if result.Core != nil || result.Editions == nil {
				t.Fatalf("malformed credits interpreted as known empty: %+v", result)
			}
		})
	}
}

func TestStaticSeparatesMainSampleAndResizedThumbnail(t *testing.T) {
	client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return response(
			200,
			`[{"workno":"RJ000001","product_name":"title","genres":[],"creaters":{},"image_main":{"url":"//img.dlsite.jp/main.webp","resize_url":"resize/main"},"image_thum":{"url":"//img.dlsite.jp/sample.jpg"}}]`,
		), nil
	})})
	result, err := client.FetchStatic(t.Context(), "RJ00000001", "ja-jp")
	if err != nil {
		t.Fatal(err)
	}
	if result.Core == nil || result.Core.CoverURL == nil ||
		*result.Core.CoverURL != "https://img.dlsite.jp/main.webp" ||
		result.Core.SampleCoverURL == nil ||
		*result.Core.SampleCoverURL != "https://img.dlsite.jp/sample.jpg" ||
		result.Core.ThumbnailURL == nil ||
		*result.Core.ThumbnailURL != "https://img.dlsite.jp/resize/main_240x240.jpg" {
		t.Fatalf("incorrect cover mapping: %+v", result.Core)
	}
}

func TestMainURLSurvivesInvalidCoreGroup(t *testing.T) {
	client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return response(
			200,
			`[{"workno":"RJ000001","product_name":"bad core","genres":[],"creaters":{"voice_by":"broken"},"image_main":{"url":"//img.dlsite.jp/main.webp"}}]`,
		), nil
	})})
	result, err := client.FetchStatic(t.Context(), "RJ00000001", "ja-jp")
	if err != nil || result.Core != nil || result.MainURL != "https://img.dlsite.jp/main.webp" {
		t.Fatalf("cover depends on valid core: %+v %v", result, err)
	}
}

func TestStaticPreservesThreeAgeCategories(t *testing.T) {
	for _, age := range []any{1, 2, 3, nil, 4, "bad"} {
		body, err := json.Marshal([]map[string]any{{"workno": "RJ000001", "product_name": "work", "age_category": age}})
		if err != nil {
			t.Fatal(err)
		}
		client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, string(body)), nil
		})})
		snapshot, err := client.FetchStatic(t.Context(), "RJ00000001", "ja-jp")
		if err != nil {
			t.Fatal(err)
		}
		value, numeric := age.(int)
		if age != nil && (!numeric || value > 3) {
			if snapshot.Core != nil {
				t.Fatalf("invalid age %v overwrites core", age)
			}
			continue
		}
		if snapshot.Core == nil {
			t.Fatalf("missing core for age %v", age)
		}
		if age == nil {
			if snapshot.Core.AgeCategory != nil {
				t.Fatal("invented an unreported age")
			}
		} else if snapshot.Core.AgeCategory == nil || *snapshot.Core.AgeCategory != value ||
			snapshot.Core.NSFW == nil || *snapshot.Core.NSFW != (value == 3) {
			t.Fatalf("incorrect age %v: %+v", age, snapshot.Core)
		}
	}
}
