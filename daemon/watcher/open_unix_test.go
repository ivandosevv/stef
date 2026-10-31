//go:build !windows

package watcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSourceFIFOOpenDoesNotBlock(t *testing.T) {
	if os.Getenv("STEF_FIFO_CHILD") == "1" {
		root, err := os.OpenRoot(os.Getenv("STEF_FIFO_FIXTURE"))
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		file, err := root.OpenFile("replaced.txt", os.O_RDONLY|watcherOpenFlags, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.Mode().IsRegular() {
			t.Fatal("a substituted FIFO was mistaken for regular source")
		}
		return
	}
	fixture := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(fixture, "replaced.txt"), 0600); err != nil {
		t.Fatal(err)
	}
	// A subprocess deadline also prevents a future blocking-open regression
	// from hanging the entire suite while waiting for a nonexistent FIFO writer.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSourceFIFOOpenDoesNotBlock$")
	cmd.Env = append(os.Environ(), "STEF_FIFO_CHILD=1", "STEF_FIFO_FIXTURE="+fixture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("source FIFO open blocked or failed: %v\n%s", err, output)
	}
}
