//go:build embedui

// Package web serves the embedded single-page application.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded UI filesystem (root = dist/).
func DistFS() (fs.FS, bool) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, false
	}
	return sub, true
}

// Handler returns the SPA handler.
func Handler() http.Handler {
	sub, _ := DistFS()
	return NewSPAHandler(sub)
}
