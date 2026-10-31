package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func fullCatalog(t *testing.T, count int) *Hub {
	t.Helper()
	h, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	for i := 1; i < count; i++ {
		id := fmt.Sprintf("retained-%d", i)
		h.sessions[id] = []telemetry.Event{{ID: fmt.Sprint(i), SessionID: id, Why: "Retain this private history."}}
	}
	return h
}

func TestNewIdentitiesFailClosedAtCatalogLimit(t *testing.T) {
	h := fullCatalog(t, MaxWorkspaces)
	if _, err := h.RegisterSession("new-chat"); err == nil {
		t.Fatal("registration bypassed the catalog cap")
	}
	if _, err := h.ActivateSession("new-chat"); err == nil {
		t.Fatal("activation bypassed the catalog cap")
	}
	if _, err := h.SetSessionUpdateMode("new-chat", telemetry.UpdatesFiles); err == nil {
		t.Fatal("new settings bypassed the catalog cap")
	}
	if _, err := h.Publish(telemetry.Event{SessionID: "new-chat", AgentID: "fixture", NodeName: "source.txt", Action: "tested", Summary: "Test admission", Why: "Reject new history at capacity.", Status: "ready"}); err == nil {
		t.Fatal("event reporting bypassed the catalog cap")
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "retained-1", Action: "create"}); err == nil {
		t.Fatal("shared workspace creation bypassed the catalog cap")
	}
	if len(h.Workspaces()) != MaxWorkspaces || len(h.sessions["retained-1"]) != 1 {
		t.Fatal("admission changed the retained catalog or private history")
	}
	if _, err := h.RegisterSession("retained-1"); err != nil {
		t.Fatalf("existing private history became unavailable: %v", err)
	}
}

func TestSharedCreationReservesOriginAndRoomBeforeRegistering(t *testing.T) {
	h := fullCatalog(t, MaxWorkspaces-1)
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "new-chat", Action: "create"}); err == nil {
		t.Fatal("creation did not reserve both new identities")
	}
	if _, exists := h.sessions["new-chat"]; exists || len(h.sessions) != MaxWorkspaces-1 {
		t.Fatal("failed creation partially registered a new origin")
	}
}

func TestOversizedCatalogReplayKeepsOriginalJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	var original []byte
	for i := 0; i < MaxWorkspaces; i++ {
		record, err := json.Marshal(sessionControl{Control: "activate", SessionID: fmt.Sprintf("old-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		original = append(original, record...)
		original = append(original, '\n')
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if h, err := New("explicit-project", path); err == nil {
		h.Close()
		t.Fatal("an oversized replay catalog was accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatal("replay rejection rewrote retained private history")
	}
}
