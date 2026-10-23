package scraper_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/Clementine33/preen/backend/internal/adapters/scraper"
)

func TestStaticPreservesSeries(t *testing.T) {
	client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK,
			`[{"workno":"RJ109959","product_name":"work","series_id":"SRI0000000198","series_name":"ささやき庵"}]`,
		), nil
	})})
	snapshot, err := client.FetchStatic(t.Context(), "RJ00109959", "ja-jp")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Core == nil || snapshot.Core.SeriesID == nil || *snapshot.Core.SeriesID != "SRI0000000198" ||
		snapshot.Core.SeriesName == nil || *snapshot.Core.SeriesName != "ささやき庵" {
		t.Fatalf("series was not mapped: %+v", snapshot.Core)
	}
}

func TestMalformedSeriesDoesNotBecomeAnEmptyCoreSnapshot(t *testing.T) {
	for _, fields := range []string{`"series_id":42`, `"series_name":{}`} {
		t.Run(fields, func(t *testing.T) {
			client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`[{"workno":"RJ109959","product_name":"work","language_editions":[],%s}]`, fields)
				return response(http.StatusOK, body), nil
			})})
			snapshot, err := client.FetchStatic(t.Context(), "RJ00109959", "ja-jp")
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Core != nil || snapshot.Editions == nil {
				t.Fatalf("malformed series cleared core or discarded valid editions: %+v", snapshot)
			}
		})
	}
}

func TestStaticWithoutSeriesKeepsCore(t *testing.T) {
	for _, fields := range []string{"", `,"series_id":null,"series_name":null`} {
		t.Run(fields, func(t *testing.T) {
			client := scraper.New(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`[{"workno":"RJ109959","product_name":"work"%s}]`, fields)
				return response(http.StatusOK, body), nil
			})})
			snapshot, err := client.FetchStatic(t.Context(), "RJ00109959", "ja-jp")
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Core == nil || snapshot.Core.SeriesID != nil || snapshot.Core.SeriesName != nil {
				t.Fatalf("unknown series prevented a valid core snapshot: %+v", snapshot.Core)
			}
		})
	}
}
