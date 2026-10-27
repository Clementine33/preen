package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAAssetsServesIndexForClientRoutes(t *testing.T) {
	files := fstest.MapFS{
		"assets/index.html":    {Data: []byte("<html>index</html>")},
		"assets/assets/app.js": {Data: []byte("app")},
	}
	for path, want := range map[string]string{"/works?rj=RJ01": "index", "/works/RJ01": "index", "/assets/app.js": "app"} {
		rec := httptest.NewRecorder()
		spaAssets(files).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Body.String())
		}
	}
}
