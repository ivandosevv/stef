package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPresentationIsRememberedAndCanBeOverridden(t *testing.T) {
	root := t.TempDir()
	if got, err := projectPresentation(root, ""); err != nil || got != "native" {
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
