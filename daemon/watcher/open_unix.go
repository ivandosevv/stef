//go:build !windows

package watcher

import (
	"os"
	"syscall"
)

// Avoid following a replaced leaf link or blocking on a substituted FIFO.
const watcherOpenFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK

func singleSourceLink(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
