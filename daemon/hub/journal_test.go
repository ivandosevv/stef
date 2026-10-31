package hub

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestJournalRefusesDirectoryAndFileLinksWithoutChangingTargets(t *testing.T) {
	for _, directoryLink := range []bool{false, true} {
		repo, outside := t.TempDir(), t.TempDir()
		marker := filepath.Join(outside, "events.jsonl")
		original := []byte("unrelated fixture contents\n")
		if err := os.WriteFile(marker, original, 0600); err != nil {
			t.Fatal(err)
		}
		dataDir := filepath.Join(repo, ".stef")
		if directoryLink {
			if err := os.Symlink(outside, dataDir); err != nil {
				t.Skipf("filesystem symlinks unavailable: %v", err)
			}
		} else {
			if err := os.Mkdir(dataDir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(marker, filepath.Join(dataDir, "events.jsonl")); err != nil {
				t.Skipf("filesystem symlinks unavailable: %v", err)
			}
		}
		if h, err := New(repo, filepath.Join(dataDir, "events.jsonl")); err == nil {
			h.Close()
			t.Fatal("linked history was accepted")
		}
		got, err := os.ReadFile(marker)
		if err != nil || string(got) != string(original) {
			t.Fatal("unrelated target was changed")
		}
	}
}

func TestJournalRejectsHardLinksAndNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "marker")
	if err := os.WriteFile(marker, []byte("keep fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "events.jsonl")
	if err := os.Link(marker, path); err != nil {
		t.Skipf("filesystem hard links unavailable: %v", err)
	}
	if h, err := New(root, path); err == nil {
		h.Close()
		t.Fatal("hard-linked history was accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if h, err := New(root, path); err == nil {
		h.Close()
		t.Fatal("directory journal was accepted")
	}
}

func TestJournalRestrictsExistingPermissionsAndKeepsExplicitDataDirectory(t *testing.T) {
	repo, directory := t.TempDir(), t.TempDir()
	path := filepath.Join(directory, "events.jsonl")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	h, err := New(repo, path)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("existing journal remained readable by other users")
	}
}

func TestJournalLimitsPreserveExistingBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := file.Truncate(MaxJournalBytes); err != nil {
		t.Fatal(err)
	}
	h := &Hub{journal: file}
	if err := h.persist([]byte("extra")); err == nil {
		t.Fatal("journal byte cap was bypassed")
	}
	info, _ := file.Stat()
	if info.Size() != MaxJournalBytes {
		t.Fatal("full journal was changed after write rejection")
	}
	if err := file.Truncate(MaxJournalBytes + 1); err != nil {
		t.Fatal(err)
	}
	if reopened, err := New("explicit-project", path); err == nil {
		reopened.Close()
		t.Fatal("oversized replay was accepted")
	}
}

func TestEscapedSourceRecordReopensWithinRecordBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := New("project", path)
	if err != nil {
		t.Fatal(err)
	}
	code := strings.Repeat(string(rune(1)), telemetry.MaxText)
	if _, err := h.Publish(telemetry.Event{AgentID: "fixture", NodeName: "source.txt", Action: "modified", Summary: "Escaped source", Why: "Keep the exact captured revision.", Status: "ready", Before: code, After: code, Diff: code}); err != nil {
		t.Fatal(err)
	}
	h.Close()
	h, err = New("project", path)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if len(h.Snapshot().Events) != 1 || h.Snapshot().Events[0].Before != code {
		t.Fatal("valid escaped source was lost on replay")
	}
}
