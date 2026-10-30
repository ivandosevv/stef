package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func workspaceByID(t *testing.T, h *Hub, id string) Workspace {
	t.Helper()
	for _, workspace := range h.Workspaces() {
		if workspace.WorkspaceID == id {
			return workspace
		}
	}
	t.Fatalf("workspace %s was not retained", id)
	return Workspace{}
}

func TestWorkspaceNamesAreReadableUniqueAndStableThroughResumeAndClear(t *testing.T) {
	h, err := New("repo", "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	pattern := regexp.MustCompile(`^[A-Z][a-z]+ [A-Z][a-z]+ [0-9]{4,}$`)
	seen := map[string]bool{}
	for i := 0; i < 128; i++ {
		id := fmt.Sprintf("chat-%03d", i)
		register(t, h, id)
		name := workspaceByID(t, h, id).Name
		if !pattern.MatchString(name) || seen[strings.ToLower(name)] {
			t.Fatalf("unreadable or duplicate name: %q", name)
		}
		seen[strings.ToLower(name)] = true
		workspaceEvent(t, h, id, "agent", "change.ts")
		register(t, h, id)
		if workspaceByID(t, h, id).Name != name || len(h.SessionSnapshot(id).Events) != 1 {
			t.Fatal("resume changed the name or discarded saved work")
		}
		if _, err := h.ClearSession(id); err != nil {
			t.Fatal(err)
		}
		if workspaceByID(t, h, id).Name != name || len(h.SessionSnapshot(id).Events) != 0 {
			t.Fatal("clear changed the name or did not clear its history")
		}
	}
}

func TestWorkspaceNameGeneratorHandlesRepeatedRandomValuesAndErrors(t *testing.T) {
	used := map[string]bool{"quiet fox 1000": true, "quiet fox 1001": true}
	name, err := generateWorkspaceName(bytes.NewReader(make([]byte, 4096)), used)
	if err != nil || name != "Quiet Fox 1002" {
		t.Fatalf("collision fallback failed: %q, %v", name, err)
	}
	if _, err := generateWorkspaceName(bytes.NewReader(nil), used); err == nil {
		t.Fatal("ignored failed local randomness")
	}
}

func TestSharingNamesResolveCaseAndWhitespaceWithoutJoiningPrivateOrAmbiguousNames(t *testing.T) {
	h, err := New("repo", "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	register(t, h, "owner")
	register(t, h, "worker")
	private := workspaceByID(t, h, "owner")
	shared, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "owner", Action: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if shared.Name == private.Name {
		t.Fatal("private and shared workspaces received the same name")
	}
	joined, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "worker", Action: "join", WorkspaceID: "  " + strings.ToLower(shared.Name) + " \n"})
	if err != nil || joined.WorkspaceID != shared.WorkspaceID || joined.Name != shared.Name {
		t.Fatalf("friendly name did not resolve to its stable shared ID: %+v, %v", joined, err)
	}
	for _, invitation := range []string{private.Name, "owner", "Missing Fox 1234", ""} {
		if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "stranger", Action: "join", WorkspaceID: invitation}); err == nil {
			t.Fatalf("accepted private or absent sharing name: %q", invitation)
		}
	}
	if _, ok := h.originFrom["stranger"]; ok {
		t.Fatal("failed invitation registered or rerouted a worker")
	}
	other, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "worker", Action: "create"})
	if err != nil {
		t.Fatal(err)
	}
	// Defensively reject ambiguous labels even if outside code corrupts the map.
	h.workspaceNames[other.WorkspaceID] = strings.ToLower(shared.Name)
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "worker", Action: "join", WorkspaceID: shared.Name}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous name did not fail: %v", err)
	}
	if h.ResolveSession("worker") != other.WorkspaceID {
		t.Fatal("ambiguous invitation changed membership")
	}
	if joined, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "worker", Action: "join", WorkspaceID: shared.WorkspaceID}); err != nil || joined.WorkspaceID != shared.WorkspaceID {
		t.Fatal("stable invitation ID stopped working")
	}
}

func TestWorkspaceNamesSurviveRestartSharingAndLeaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	register(t, h, "owner")
	privateName := workspaceByID(t, h, "owner").Name
	privateEvent := workspaceEvent(t, h, "owner", "owner-agent", "private.ts")
	shared, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "owner", Action: "create"})
	if err != nil {
		t.Fatal(err)
	}
	workspaceEvent(t, h, "owner", "owner-agent", "shared.ts")
	if _, err := h.ClearSession("owner"); err != nil {
		t.Fatal(err)
	}
	sharedEvent := workspaceEvent(t, h, "owner", "owner-agent", "shared-again.ts")
	before := h.Workspaces()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if !reflect.DeepEqual(before, h.Workspaces()) {
		t.Fatal("names or memberships changed on restart")
	}
	register(t, h, "owner")
	if h.ResolveSession("owner") != shared.WorkspaceID || workspaceByID(t, h, shared.WorkspaceID).Name != shared.Name {
		t.Fatal("ordinary registration reset active sharing or its name")
	}
	left, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "owner", Action: "leave"})
	if err != nil || left.Name != privateName || left.Shared {
		t.Fatalf("leaving did not restore original private label: %+v, %v", left, err)
	}
	if got := h.SessionSnapshot("owner").Events; len(got) != 1 || got[0].ID != privateEvent.ID {
		t.Fatal("leaving did not restore exact private history")
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "owner", Action: "join", WorkspaceID: shared.Name}); err != nil {
		t.Fatal(err)
	}
	if got := h.SessionSnapshot("owner").Events; len(got) != 1 || got[0].ID != sharedEvent.ID {
		t.Fatal("friendly invitation did not restore exact shared history")
	}
}

func TestLegacyNameMigrationIsAppendOnlyAndDoesNotRepeatOnRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	register(t, h, "owner")
	privateEvent := workspaceEvent(t, h, "owner", "owner-agent", "private.ts")
	shared, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "owner", Action: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "peer", Action: "join", WorkspaceID: shared.WorkspaceID}); err != nil {
		t.Fatal(err)
	}
	workspaceObserve(t, h, "local", "observed.ts", "file")
	if err := h.ClaimChanges("peer", []string{"observed.ts"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SetSessionUpdateMode("owner", telemetry.UpdatesMilestones); err != nil {
		t.Fatal(err)
	}
	exact := h.SessionSnapshot("peer")
	exactEvents, err := json.Marshal(exact.Events)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Remove only fields introduced in this version to reconstruct an old journal.
	var old bytes.Buffer
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{10}) {
		var record map[string]json.RawMessage
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		if string(record["stef_control"]) == `"workspace_name"` {
			continue
		}
		delete(record, "workspace_name")
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		old.Write(encoded)
		old.WriteByte(10)
	}
	if err := os.WriteFile(path, old.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	h, err = New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	got := h.SessionSnapshot("peer")
	savedEvents, err := json.Marshal(got.Events)
	if err != nil || !bytes.Equal(savedEvents, exactEvents) || got.UpdateMode != exact.UpdateMode || got.UpdatesFrom != exact.UpdatesFrom || got.WorkspaceID != exact.WorkspaceID {
		t.Fatal("migration changed saved revisions, settings, or shared routing")
	}
	for _, room := range h.Workspaces() {
		if room.Name == "" {
			t.Fatal("legacy workspace did not receive a name")
		}
	}
	names := h.Workspaces()
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(migrated, old.Bytes()) || len(migrated) <= old.Len() {
		t.Fatalf("migration rewrote history or did not append labels: %v", err)
	}
	h, err = New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if !reflect.DeepEqual(names, h.Workspaces()) {
		t.Fatal("legacy labels changed after a second restart")
	}
	reopened, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(migrated, reopened) {
		t.Fatal("completed migration appended duplicate labels")
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "owner", Action: "leave"}); err != nil {
		t.Fatal(err)
	}
	if got := h.SessionSnapshot("owner").Events; len(got) != 1 || got[0].ID != privateEvent.ID {
		t.Fatal("migration lost the private history behind shared membership")
	}
}
