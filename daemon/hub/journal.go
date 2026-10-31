package hub

import (
	"fmt"
	"os"
	"path/filepath"
)

const MaxJournalBytes int64 = 256 * 1024 * 1024
const MaxJournalRecord = 1024 * 1024

// The default directory belongs to the selected project. Explicit custom data
// directories are intentional locations, but their journal must still be a
// private regular file, never a symlink, hard link, device or pipe.
func openJournal(repo, path string) (*os.File, error) {
	defaultPath, err := filepath.Abs(filepath.Join(repo, ".stef", "events.jsonl"))
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	var directory *os.Root
	if absolute == defaultPath {
		root, err := os.OpenRoot(repo)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		info, err := root.Lstat(".stef")
		if os.IsNotExist(err) {
			if err := root.Mkdir(".stef", 0700); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(".stef history directory must be a project-owned directory without links")
		}
		directory, err = root.OpenRoot(".stef")
		if err != nil {
			return nil, err
		}
		entry, entryErr := root.Lstat(".stef")
		opened, openErr := directory.Stat(".")
		if entryErr != nil || openErr != nil || !entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 || !os.SameFile(entry, opened) {
			directory.Close()
			return nil, fmt.Errorf(".stef history directory changed during opening")
		}
		if err := directory.Chmod(".", 0700); err != nil {
			directory.Close()
			return nil, err
		}
	} else {
		info, err := os.Lstat(filepath.Dir(absolute))
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("history directory must be a directory without links")
		}
		directory, err = os.OpenRoot(filepath.Dir(absolute))
		if err != nil {
			return nil, err
		}
		opened, err := directory.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			directory.Close()
			return nil, fmt.Errorf("history directory changed during opening")
		}
	}
	defer directory.Close()
	name := filepath.Base(absolute)
	if info, err := directory.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("history journal must be a regular file without links")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	file, err := directory.OpenFile(name, os.O_CREATE|os.O_RDWR|journalOpenFlags, 0600)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*os.File, error) { file.Close(); return nil, err }
	info, err := file.Stat()
	if err != nil {
		return fail(err)
	}
	entry, err := directory.Lstat(name)
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() || !entry.Mode().IsRegular() || !os.SameFile(info, entry) {
		return fail(fmt.Errorf("history journal was replaced or is not a regular file"))
	}
	if err := validateJournalHandle(file); err != nil {
		return fail(err)
	}
	if info.Size() > MaxJournalBytes {
		return fail(fmt.Errorf("history exceeds the %d MiB journal limit; archive it or use --no-persist", MaxJournalBytes/(1024*1024)))
	}
	if err := file.Chmod(0600); err != nil {
		return fail(err)
	}
	return file, nil
}
