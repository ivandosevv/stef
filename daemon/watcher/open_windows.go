package watcher

import (
	"os"
	"syscall"
)

// Windows os.Root confines the open; watcher.read verifies the resulting
// regular file against its Lstat entry before reading source contents.
const watcherOpenFlags = 0

func singleSourceLink(file *os.File) bool {
	var info syscall.ByHandleFileInformation
	return syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info) == nil && info.NumberOfLinks == 1
}
