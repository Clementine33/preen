package httpapi

import (
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
)

var hashedWebAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{8,}\.[^.]+$`)

// webMiddleware keeps the public application outside the API's routing and auth.
func webMiddleware(root *os.Root, api http.Handler) http.Handler {
	if root == nil {
		return api
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if strings.ContainsAny(name, "\\\x00") {
			http.NotFound(w, r)
			return
		}
		for segment := range strings.SplitSeq(name, "/") {
			if segment == ".." || segment == "." {
				http.NotFound(w, r)
				return
			}
		}
		if name == "" {
			name = "index.html"
		}
		info, err := root.Stat(name)
		if os.IsNotExist(err) || (err == nil && info.IsDir()) {
			name = "index.html"
		}
		file, err := root.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		if hashedWebAsset.MatchString(path.Base(name)) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeContent(w, r, name, info.ModTime(), file)
	})
}
