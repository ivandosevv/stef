package hub

import (
	"fmt"
	"os"
	"syscall"
)

// os.Root confines Windows opens; Lstat and the opened handle are compared
// before any contents are read, and reparse points are rejected by openJournal.
const journalOpenFlags = 0

func validateJournalHandle(file *os.File) error {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks != 1 {
		return fmt.Errorf("history journal must have exactly one filesystem link")
	}
	return nil
}
