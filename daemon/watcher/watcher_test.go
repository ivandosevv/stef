package watcher_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
	"github.com/ivandosevv/stef/daemon/watcher"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func await(t *testing.T, messages <-chan hub.Message, node, action string) telemetry.Event {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case message := <-messages:
			if message.Event != nil && message.Event.NodeName == node && message.Event.Action == action {
				return *message.Event
			}
		case <-timeout.C:
			t.Fatalf("timed out awaiting %s %s", action, node)
		}
	}
}

func TestFileTrackingASTAtomicSaveAndDirectoryRemoval(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	write(t, path, "package sample\nfunc Run() int { return 1 }\n")
	h, err := hub.New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	w, err := watcher.New(root, h)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	messages, unsubscribe := h.Subscribe()
	defer unsubscribe()
	<-messages
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- w.Run(ctx) }()
	write(t, filepath.Join(root, "temporary.go"), "package sample\nfunc Run() int { return 2 }\n")
	if err := os.Rename(filepath.Join(root, "temporary.go"), path); err != nil {
		t.Fatal(err)
	}
	event := await(t, messages, "main.go#Run", "modified")
	if !strings.Contains(event.Diff, "-func Run() int { return 1 }") || !strings.Contains(event.Diff, "+func Run() int { return 2 }") {
		t.Fatalf("incorrect AST diff: %s", event.Diff)
	}
	if event.Source != "watcher" || event.AgentID != "filesystem" {
		t.Fatal("filesystem observation misattributed")
	}
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(nested, "new.txt"), "hello\n")
	await(t, messages, "nested/new.txt", "created")
	if err := os.RemoveAll(nested); err != nil {
		t.Fatal(err)
	}
	await(t, messages, "nested/new.txt", "deleted")
	cancel()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestIgnoredFilesAndSymlinksStayOutsideTheStream(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	cmd := exec.Command("git", "init", root)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	if err := os.Mkdir(filepath.Join(root, "ignored"), 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "ignored", "secret.txt"), "baseline")
	write(t, filepath.Join(outside, "external.txt"), "outside")
	if err := os.Symlink(outside, filepath.Join(root, "external")); err != nil {
		t.Fatal(err)
	}
	h, err := hub.New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	w, err := watcher.New(root, h)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	messages, unsubscribe := h.Subscribe()
	defer unsubscribe()
	<-messages
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	write(t, filepath.Join(root, "ignored", "secret.txt"), "changed")
	write(t, filepath.Join(root, "external", "external.txt"), "changed outside")
	write(t, filepath.Join(root, "visible.txt"), "visible")
	await(t, messages, "visible.txt", "created")
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for _, e := range h.Snapshot().Events {
		if strings.Contains(e.NodeName, "ignored") || strings.Contains(e.NodeName, "external") {
			t.Fatalf("tracked excluded file: %s", e.NodeName)
		}
	}
}

func TestParserSeparatesMethodsAndTypes(t *testing.T) {
	symbols := watcher.Extract("sample.go", []byte("package sample\ntype A struct{}\ntype B struct{}\nfunc (a *A) Run() {}\nfunc (b *B) Run() {}\n"))
	for _, node := range []string{"sample.go#A", "sample.go#B", "sample.go#*A.Run", "sample.go#*B.Run"} {
		if _, ok := symbols[node]; !ok {
			t.Errorf("missing %s", node)
		}
	}
}
