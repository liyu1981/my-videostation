package web

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// NewSPAHandler serves static files from fsys and falls back to index.html for
// client-side routes. Requests with a file extension that do not exist return 404
// so broken assets fail loudly instead of returning HTML.
func NewSPAHandler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		upath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		name := strings.TrimPrefix(upath, "/")
		if name == "" {
			name = "index.html"
		}

		if f, err := fsys.Open(name); err == nil {
			defer f.Close()
			if st, err := f.Stat(); err == nil && !st.IsDir() {
				w.Header().Set("X-Content-Type-Options", "nosniff")
				if strings.HasPrefix(name, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				http.ServeContent(w, r, name, st.ModTime(), f.(io.ReadSeeker))
				return
			}
		}

		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}

		index, err := fsys.Open("index.html")
		if err != nil {
			http.Error(w, "UI not embedded in this build", http.StatusNotFound)
			return
		}
		defer index.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodHead {
			return
		}
		http.ServeContent(w, r, "index.html", time.Time{}, index.(io.ReadSeeker))
	})
}
