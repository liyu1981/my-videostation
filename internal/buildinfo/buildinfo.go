// Package buildinfo carries version information injected at build time.
package buildinfo

// Overridden with -ldflags "-X .../internal/buildinfo.Version=..." etc.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
