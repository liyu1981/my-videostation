//go:build !windows

package scanner

import (
	"io/fs"
	"syscall"
)

// inodeOf returns the file inode on Unix-like systems (0 when unavailable).
func inodeOf(st fs.FileInfo) int64 {
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		return int64(sys.Ino)
	}
	return 0
}
