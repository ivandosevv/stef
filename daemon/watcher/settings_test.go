package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestSuppressedSavesAdvanceBaselineWithoutDeferredChanges(t *testing.T) {
	for _, mode := range []telemetry.UpdateMode{telemetry.UpdatesMilestones, telemetry.UpdatesDisabled} {
		t.Run(string(mode), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "main.ts")
			removed := filepath.Join(root, "removed.ts")
			created := filepath.Join(root, "created.ts")
			write := func(name, value string) {
				t.Helper()
				if err := os.WriteFile(name, []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(path, "original\n")
			write(removed, "original\n")
			h, err := hub.New(root, "")
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			w, err := New(root, h)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			if _, err := h.SetUpdateMode(mode); err != nil {
				t.Fatal(err)
			}
			write(path, "suppressed edit\n")
			write(created, "created while suppressed\n")
			if err := os.Remove(removed); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{path, created, removed} {
				if err := w.process(name); err != nil {
					t.Fatal(err)
				}
			}
			if len(h.Snapshot().Events) != 0 {
				t.Fatal("suppressed saves entered retained history")
			}
			if _, err := h.SetUpdateMode(telemetry.UpdatesFiles); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{path, created, removed} {
				if err := w.process(name); err != nil {
					t.Fatal(err)
				}
			}
			if len(h.Snapshot().Events) != 0 {
				t.Fatal("enabling file mode replayed suppressed creates, edits, or deletes")
			}
			write(path, "visible edit\n")
			if err := w.process(path); err != nil {
				t.Fatal(err)
			}
			events := h.Snapshot().Events
			if len(events) != 1 || events[0].NodeName != "main.ts" || events[0].Before != "suppressed edit\n" || events[0].After != "visible edit\n" {
				t.Fatalf("next save compared against the wrong baseline: %+v", events)
			}
		})
	}
}

func TestQueuedSuppressedSaveDoesNotCaptureAfterModeChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.ts")
	if err := os.WriteFile(path, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := hub.New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	w, err := New(root, h)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := h.SetUpdateMode(telemetry.UpdatesDisabled); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("queued while disabled\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SetUpdateMode(telemetry.UpdatesFiles); err != nil {
		t.Fatal(err)
	}
	// The queued save was observed with capture disabled before debounce finished.
	if err := w.processWithCapture(path, false); err != nil {
		t.Fatal(err)
	}
	if err := w.process(path); err != nil {
		t.Fatal(err)
	}
	if len(h.Snapshot().Events) != 0 {
		t.Fatal("a queued disabled save was revived on enable")
	}
}
