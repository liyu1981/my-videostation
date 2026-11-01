//go:build windows

package scanner

import "io/fs"

// inodeOf is not available on Windows; file identity falls back to the path.
func inodeOf(fs.FileInfo) int64 { return 0 }
