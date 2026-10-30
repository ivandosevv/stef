package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionExportConfinedToProject(t *testing.T) {
	root := t.TempDir()
	save := sessionExporter(root)
	name, err := save(`{"version":1,"events":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(name) != filepath.Join(root, ".stef", "exports") {
		t.Fatal("export outside project")
	}
	if data, err := os.ReadFile(name); err != nil || string(data) != `{"version":1,"events":[]}` {
		t.Fatal("export missing")
	}
	if _, err := save("not JSON"); err == nil {
		t.Fatal("accepted invalid export")
	}
	otherRoot, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(otherRoot, ".stef")); err != nil {
		t.Fatal(err)
	}
	if _, err := sessionExporter(otherRoot)(`{"version":1}`); err == nil {
		t.Fatal("symlink escaped export root")
	}
	if files, err := os.ReadDir(outside); err != nil || len(files) != 0 {
		t.Fatal("wrote through outside symlink")
	}
}
