//go:build !embedui

package web

import (
	"io/fs"
	"net/http"
)

// DistFS reports that no UI was embedded in this build.
func DistFS() (fs.FS, bool) { return nil, false }

// Handler serves a stub page until the real UI is embedded (build tag `embedui`).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!doctype html>
<html><head><meta charset="utf-8"><title>MyVideoStation</title>
<style>
  body{margin:0;min-height:100vh;display:grid;place-items:center;background:#14161a;color:#f2f4f6;
       font:15px/1.6 ui-sans-serif,system-ui,sans-serif}
  code{background:#242a33;padding:2px 6px;border-radius:4px}
  a{color:#22d3ee}
</style></head>
<body><div>
  <h1 style="font-size:20px;margin:0 0 8px">MyVideoStation API is running</h1>
  <p style="color:#a9b1bb;margin:0 0 16px">
    The web UI was not embedded in this build. Run <code>make build</code>
    (or <code>make dev</code> for the Vite dev server).
  </p>
  <p style="color:#737b86;margin:0">API: <a href="/api/v1/health">/api/v1/health</a></p>
</div></body></html>`))
	})
}
