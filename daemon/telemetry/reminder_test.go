package telemetry_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestReminderAcceptsOnlyCanonicalPlainFilePaths(t *testing.T) {
	for _, value := range []string{"", "@workflow", "main.go#Run", "/main.go", "../main.go", "src/../main.go", "./main.go", "src//main.go", "src/", " main.go", "main.go ", "src\\main.go", "C:/main.go", "main\t.go", "main\n.go", string([]byte{'a', 255})} {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			if _, err := telemetry.ReminderPaths([]string{value}); err == nil {
				t.Fatalf("invalid path accepted: %q", value)
			}
		})
	}
	if _, err := telemetry.ReminderPaths(nil); err == nil {
		t.Fatal("empty path list accepted")
	}
	paths := []string{"src/my file.go", ".config/stef.json", "src/テーマ.ts"}
	for i := 0; i < 70; i++ {
		paths = append(paths, "src/my file.go")
	}
	if files, err := telemetry.ReminderPaths(paths); err != nil || len(files) != 3 {
		t.Fatalf("valid paths or duplicates rejected: %v %v", files, err)
	}
	paths = nil
	for i := 0; i < 50; i++ {
		paths = append(paths, fmt.Sprintf("file-%d.go", i))
	}
	if _, err := telemetry.ReminderPaths(paths); err != nil {
		t.Fatal("fifty files rejected", err)
	}
	if _, err := telemetry.ReminderPaths(append(paths, "other.go")); err == nil {
		t.Fatal("fifty-one distinct files accepted")
	}
}

func TestReminderRequestValidatesSessionAndBoundedWait(t *testing.T) {
	for _, session := range []string{" chat", "chat ", "chat\nother", strings.Repeat("a", 161)} {
		if err := (telemetry.ReminderRequest{SessionID: session, Paths: []string{"main.go"}}).Validate(); err == nil {
			t.Fatalf("invalid session accepted: %q", session)
		}
	}
	for _, wait := range []int{-1, 1001} {
		if err := (telemetry.ReminderRequest{Paths: []string{"main.go"}, SettleMS: wait}).Validate(); err == nil {
			t.Fatal("invalid wait accepted", wait)
		}
	}
	for _, wait := range []int{0, 300, 1000} {
		if err := (telemetry.ReminderRequest{Paths: []string{"main.go"}, SettleMS: wait}).Validate(); err != nil {
			t.Fatal("valid request rejected", err)
		}
	}
}
