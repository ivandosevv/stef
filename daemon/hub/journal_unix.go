//go:build !windows

package hub

import (
	"fmt"
	"os"
	"syscall"
)

const journalOpenFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK

func validateJournalHandle(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Nlink != 1 {
		return fmt.Errorf("history journal must have exactly one filesystem link")
	}
	return nil
}
