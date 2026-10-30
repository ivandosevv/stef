package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPresentationIsRememberedAndCanBeOverridden(t *testing.T) {
	root := t.TempDir()
	if got, err := projectPresentation(root, ""); err != nil || got != "ide" {
		t.Fatalf("fresh project = %q, %v", got, err)
	}
	if err := savePresentation(root, "ide"); err != nil {
		t.Fatal(err)
	}
	if got, err := projectPresentation(root, ""); err != nil || got != "ide" {
		t.Fatalf("remembered presentation = %q, %v", got, err)
	}
	if got, err := projectPresentation(root, "native"); err != nil || got != "native" {
		t.Fatalf("explicit override = %q, %v", got, err)
	}
	if _, err := projectPresentation(root, "unknown"); err == nil {
		t.Fatal("accepted an unknown host")
	}
	if err := os.WriteFile(filepath.Join(root, ".stef/presentation.json"), []byte(`{"view":"unknown"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := projectPresentation(root, ""); err == nil {
		t.Fatal("silently ignored invalid saved presentation")
	}
	if got, err := projectPresentation(root, "ide"); err != nil || got != "ide" {
		t.Fatalf("explicit recovery = %q, %v", got, err)
	}
}

func TestUnavailableNativePresentationFallsBackToIDE(t *testing.T) {
	for _, available := range []bool{false, true} {
		if got := availablePresentation("ide", available); got != "ide" {
			t.Fatalf("IDE choice changed with native availability: %q", got)
		}
		want := "ide"
		if available {
			want = "native"
		}
		if got := availablePresentation("native", available); got != want {
			t.Fatalf("native available=%v: got %q, want %q", available, got, want)
		}
	}
}

func TestDaemonExecutableAliasAcceptsWindowsSuffix(t *testing.T) {
	for _, name := range []string{"stefd", "stefd.exe", "STEFd.EXE"} {
		if !daemonExecutableName(name) {
			t.Fatalf("daemon alias was not recognized: %s", name)
		}
	}
	for _, name := range []string{"stef", "stef.exe", "stefd-other.exe"} {
		if daemonExecutableName(name) {
			t.Fatalf("ordinary executable was treated as a daemon alias: %s", name)
		}
	}
}

func TestPresentationCannotReadOrWriteOutsideProject(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".stef")); err != nil {
		t.Skip(err)
	}
	if err := savePresentation(root, "ide"); err == nil {
		t.Fatal("wrote through an escaping .stef symlink")
	}
	if err := os.WriteFile(filepath.Join(outside, "presentation.json"), []byte(`{"view":"ide"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := projectPresentation(root, ""); err == nil {
		t.Fatal("read through an escaping .stef symlink")
	}
}
