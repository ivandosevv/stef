package watcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
)

func TestSensitivePathsAreIndependentOfGit(t *testing.T) {
	for _, useGit := range []bool{false, true} {
		root := t.TempDir()
		if useGit {
			if err := exec.Command("git", "init", root).Run(); err != nil {
				t.Fatal(err)
			}
		}
		paths := []string{".env", ".env.local", "nested/.ENV.production", ".aws/credentials", ".ssh/id_ed25519", "server.pem", ".npmrc", ".codex/config.toml", ".mcp.json"}
		for _, name := range paths {
			path := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("fixture credentials"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if useGit {
			if err := exec.Command("git", "-C", root, "add", ".").Run(); err != nil {
				t.Fatal(err)
			}
		}
		h, err := hub.New(root, "")
		if err != nil {
			t.Fatal(err)
		}
		w, err := New(root, h)
		if err != nil {
			h.Close()
			t.Fatal(err)
		}
		for _, name := range paths {
			if _, ok := w.relative(filepath.Join(root, filepath.FromSlash(name))); ok {
				t.Fatalf("credential path is observable: %s", name)
			}
			if err := w.process(filepath.Join(root, filepath.FromSlash(name))); err != nil {
				t.Fatal(err)
			}
		}
		if len(w.files) != 0 || len(h.Snapshot().Events) != 0 {
			t.Fatal("credential paths entered the baseline or workflow")
		}
		w.Close()
		h.Close()
	}
}

func TestCredentialRevisionNeverReturnsInLaterDiffs(t *testing.T) {
	markers := []string{"-----BEGIN " + "PRIVATE KEY-----", "ghp_" + strings.Repeat("a", 36), "sk-proj-" + strings.Repeat("b", 36), "AKIA" + strings.Repeat("C", 16)}
	for _, secret := range markers {
		root := t.TempDir()
		path := filepath.Join(root, "source.txt")
		if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
			t.Fatal(err)
		}
		h, err := hub.New(root, "")
		if err != nil {
			t.Fatal(err)
		}
		w, err := New(root, h)
		if err != nil {
			t.Fatal(err)
		}
		if len(w.files) != 0 {
			t.Fatal("secret revision entered the baseline")
		}
		if err := os.WriteFile(path, []byte("public source"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := w.process(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
			t.Fatal(err)
		}
		if err := w.process(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := w.process(path); err != nil {
			t.Fatal(err)
		}
		for _, event := range h.Snapshot().Events {
			if strings.Contains(event.Before+event.After+event.Diff, secret) {
				t.Fatal("suppressed credential reappeared in a retained diff")
			}
		}
		if len(h.Snapshot().Events) != 1 {
			t.Fatal("secret/deletion emitted an extra revision")
		}
		w.Close()
		h.Close()
	}
}

func TestWatchedDirectoryLimitFailsBeforeRegisteringMore(t *testing.T) {
	root := t.TempDir()
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
	for len(w.dirs) < MaxDirectories {
		w.dirs["fixture-"+strconv.Itoa(len(w.dirs))] = true
	}
	newDir := filepath.Join(root, "new")
	if err := os.Mkdir(newDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := w.scan(newDir, false); err == nil {
		t.Fatal("unbounded directory watch registration")
	}
}

func TestBaselineBudgetAndComparisonWorkStayBounded(t *testing.T) {
	w := &Watcher{files: make(map[string]map[string]Symbol), baselineBytes: MaxBaselineBytes}
	if err := w.checkBaseline("large.txt", map[string]Symbol{"large.txt": {Code: "extra"}}); err == nil {
		t.Fatal("baseline byte limit was bypassed")
	}
	w.baselineBytes = 0
	current := map[string]Symbol{"public.txt": {Code: "public"}}
	w.replaceBaseline("public.txt", current)
	if w.baselineBytes != 6 {
		t.Fatal("baseline size was not recorded")
	}
	w.replaceBaseline("public.txt", nil)
	if w.baselineBytes != 0 || len(w.files) != 0 {
		t.Fatal("forgetting a revision retained its bytes")
	}
	if result := Diff("many.txt", strings.Repeat("before\n", 4001), strings.Repeat("after\n", 4001)); !strings.Contains(result, "safety limit") {
		t.Fatal("unbounded local diff comparison was accepted")
	}
}

func TestReadRefusesInternalDirectoryAliases(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, ".aws")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "settings.txt"), []byte("private fixture contents"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "public")
	if err := os.Symlink(private, alias); err != nil {
		t.Skipf("filesystem symlinks unavailable: %v", err)
	}
	w := &Watcher{root: root}
	if data, err := w.read(filepath.Join(alias, "settings.txt")); err != nil || data != nil {
		t.Fatal("an internal directory alias exposed excluded contents")
	}
}

func TestReadRefusesHardLinkedCredentialAliases(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, ".env")
	if err := os.WriteFile(private, []byte("private fixture contents"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "public.txt")
	if err := os.Link(private, alias); err != nil {
		t.Skipf("filesystem hard links unavailable: %v", err)
	}
	w := &Watcher{root: root}
	if data, err := w.read(alias); err != nil || data != nil {
		t.Fatal("a hard-linked alias exposed excluded contents")
	}
}
