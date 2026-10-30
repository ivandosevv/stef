package watcher_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ivandosevv/stef/daemon/telemetry"
	"github.com/ivandosevv/stef/daemon/watcher"
)

func TestUnifiedDiffPreservesExactLines(t *testing.T) {
	const missingNewline = "\\ No newline at end of file\n"
	cases := []struct {
		name, before, after, hunk string
	}{
		{"new file", "", "hello\n", "@@ -0,0 +1 @@\n+hello\n"},
		{"deleted file", "hello\n", "", "@@ -1 +0,0 @@\n-hello\n"},
		{"one blank line added", "", "\n", "@@ -0,0 +1 @@\n+\n"},
		{"one blank line removed", "\n", "", "@@ -1 +0,0 @@\n-\n"},
		{"empty unchanged", "", "", ""},
		{"terminated unchanged", "hello\n", "hello\n", ""},
		{"unterminated unchanged", "hello", "hello", ""},
		{"both missing newline", "old", "new", "@@ -1 +1 @@\n-old\n" + missingNewline + "+new\n" + missingNewline},
		{"add final newline", "same", "same\n", "@@ -1 +1 @@\n-same\n" + missingNewline + "+same\n"},
		{"remove final newline", "same\n", "same", "@@ -1 +1 @@\n-same\n+same\n" + missingNewline},
		{"missing newline context", "old\nkeep", "new\nkeep", "@@ -1,2 +1,2 @@\n-old\n+new\n keep\n" + missingNewline},
		{"CRLF", "old\r\n", "new\r\n", "@@ -1 +1 @@\n-old\r\n+new\r\n"},
		{"Unicode", "Здравей, Иван!\n", "Здравей, Иване!\n", "@@ -1 +1 @@\n-Здравей, Иван!\n+Здравей, Иване!\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.hunk
			if want != "" {
				want = "--- a/file.txt\n+++ b/file.txt\n" + want
			}
			if got := watcher.Diff("file.txt", tc.before, tc.after); got != want {
				t.Fatalf("diff does not preserve the recorded lines:\n got %q\nwant %q", got, want)
			}
		})
	}
}

func TestUnifiedDiffMatchesGitHunks(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name, before, after string
	}{
		{"create", "", "hello\n"},
		{"delete", "hello\n", ""},
		{"blank line", "", "\n"},
		{"both missing LF", "old", "new"},
		{"add LF", "same", "same\n"},
		{"remove LF", "same\n", "same"},
		{"unterminated context", "old\nkeep", "new\nkeep"},
		{"Unicode and CRLF", "Иван\r\n", "Иване\r\n"},
		{"separate hunks", "a\n1\n2\n3\n4\n5\n6\n7\n8\nz\n", "b\n1\n2\n3\n4\n5\n6\n7\n8\ny\n"},
	}
	hunks := func(diff string) string {
		if start := strings.Index(diff, "@@ "); start >= 0 {
			lines := strings.Split(diff[start:], "\n")
			for i, line := range lines {
				// Git may append a language-dependent context label to a hunk.
				if strings.HasPrefix(line, "@@ ") {
					header, _, _ := strings.Cut(line, " @@")
					lines[i] = header + " @@"
				}
			}
			return strings.Join(lines, "\n")
		}
		return ""
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			write(t, filepath.Join(root, "before.txt"), tc.before)
			write(t, filepath.Join(root, "after.txt"), tc.after)
			cmd := exec.Command("git", "diff", "--no-index", "--no-color", "--no-ext-diff", "--text", "--", "before.txt", "after.txt")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
			reference, err := cmd.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); err != nil && (!ok || exit.ExitCode() != 1) {
				t.Fatalf("git diff failed: %v\n%s", err, reference)
			}
			if got, want := hunks(watcher.Diff("file.txt", tc.before, tc.after)), hunks(string(reference)); got != want {
				t.Fatalf("saved revision differs from git diff:\n got %q\nwant %q", got, want)
			}
		})
	}
}

func TestDiffTruncationIsBoundedExplicitAndUTF8Safe(t *testing.T) {
	text := strings.Repeat("Иване\n", telemetry.MaxText/2)
	diff := watcher.Diff("long.txt", "", text)
	if len(diff) > telemetry.MaxText || !utf8.ValidString(diff) {
		t.Fatalf("invalid bounded diff: bytes=%d, utf8=%t", len(diff), utf8.ValidString(diff))
	}
	if !strings.HasSuffix(diff, "\n… [truncated]") {
		t.Fatal("a partial diff must retain its explicit truncation marker")
	}
	if !strings.HasPrefix(diff, "--- a/long.txt\n+++ b/long.txt\n@@ -0,0 +") {
		t.Fatal("a truncated creation must retain its real empty-side hunk range")
	}
	limit := strings.Repeat("a", telemetry.MaxText)
	if watcher.Clip(limit) != limit {
		t.Fatal("text exactly at the cap was unnecessarily changed")
	}
}
