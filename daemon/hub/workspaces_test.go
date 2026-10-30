package hub

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func register(t *testing.T, h *Hub, id string) {
	t.Helper()
	if _, err := h.RegisterSession(id); err != nil {
		t.Fatal(err)
	}
}
func workspaceEvent(t *testing.T, h *Hub, session, agent, node string) telemetry.Event {
	t.Helper()
	event, err := h.Publish(telemetry.Event{SessionID: session, AgentID: agent, NodeName: node, Action: "modified", Summary: "Update file", Why: "Keep this change attributed to its actual contributing agent.", Status: "ready", Source: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
func nextWorkspaceMessage(t *testing.T, stream <-chan Message, kind string) Message {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case message := <-stream:
			if message.Type == kind {
				return message
			}
		case <-deadline:
			t.Fatalf("missing %s message", kind)
			return Message{}
		}
	}
}
func drainWorkspaceStream(stream <-chan Message) {
	for {
		select {
		case <-stream:
		default:
			return
		}
	}
}

func TestPrivateWorkspacesKeepEventsViewsSettingsAndNotesSeparate(t *testing.T) {
	h, _ := New("repo", "")
	defer h.Close()
	register(t, h, "codex-chat")
	register(t, h, "claude-chat")
	codex, stopCodex := h.SubscribeSession("codex-chat")
	defer stopCodex()
	claude, stopClaude := h.SubscribeSession("claude-chat")
	defer stopClaude()
	if (<-codex).SessionID != "codex-chat" || (<-claude).SessionID != "claude-chat" {
		t.Fatal("views did not pin to their own origins")
	}
	first := workspaceEvent(t, h, "codex-chat", "codex", "one.ts")
	if got := nextWorkspaceMessage(t, codex, "event"); got.Event.ID != first.ID {
		t.Fatal("own event missing")
	}
	select {
	case got := <-claude:
		t.Fatalf("private event leaked: %+v", got)
	default:
	}
	if _, err := h.SetSessionUpdateMode("codex-chat", telemetry.UpdatesMilestones); err != nil {
		t.Fatal(err)
	}
	if h.SessionSnapshot("claude-chat").UpdateMode != telemetry.UpdatesFiles {
		t.Fatal("settings leaked into other workspace")
	}
	note, _ := (telemetry.Feedback{SessionID: "codex-chat", TargetAgent: "*", Kind: "suggestion", Text: "check this choice"}).Event()
	if _, err := h.Publish(note); err != nil {
		t.Fatal(err)
	}
	if len(h.OfferFeedback("claude", "claude-chat")) != 0 || len(h.OfferFeedback("codex", "codex-chat")) != 1 {
		t.Fatal("notes crossed workspace boundaries")
	}
	if _, err := h.ClearSession("claude-chat"); err != nil {
		t.Fatal(err)
	}
	if len(h.SessionSnapshot("codex-chat").Events) != 2 {
		t.Fatal("other workspace clear discarded private work")
	}
}
func TestCaptureGenerationChangesOnlyWhenAggregateObservationCrossesBoundary(t *testing.T) {
	h, _ := New("repo", "")
	defer h.Close()
	register(t, h, "one")
	register(t, h, "two")
	_, initial := h.CaptureState()
	h.SetSessionUpdateMode("one", telemetry.UpdatesDisabled)
	mode, generation := h.CaptureState()
	if mode != telemetry.UpdatesFiles || generation != initial {
		t.Fatal("one workspace dropped another workspace queued observations")
	}
	h.SetSessionUpdateMode("two", telemetry.UpdatesDisabled)
	mode, stopped := h.CaptureState()
	if mode != telemetry.UpdatesDisabled || stopped <= generation {
		t.Fatal("stopping observation retained the queued capture generation")
	}
	register(t, h, "three")
	mode, resumed := h.CaptureState()
	if mode != telemetry.UpdatesFiles || resumed <= stopped {
		t.Fatal("new private workspace revived old queued saves")
	}
}

func TestSharedWorkspaceStartsEmptyKeepsAuthorsAndRestoresPrivateHistories(t *testing.T) {
	h, _ := New("repo", "")
	defer h.Close()
	register(t, h, "parent")
	register(t, h, "child")
	private := workspaceEvent(t, h, "parent", "parent-agent", "private.ts")
	childPrivate := workspaceEvent(t, h, "child", "subagent", "child-private.ts")
	view, stop := h.SubscribeSession("parent")
	defer stop()
	<-view
	shared, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "parent", Action: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if !shared.Shared || len(h.SessionSnapshot("parent").Events) != 0 {
		t.Fatal("shared room copied private history")
	}
	if got := nextWorkspaceMessage(t, view, "snapshot"); got.SessionID != shared.WorkspaceID {
		t.Fatal("origin view did not follow its own sharing choice")
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "child", Action: "join", WorkspaceID: shared.WorkspaceID}); err != nil {
		t.Fatal(err)
	}
	third := "other-agent"
	register(t, h, third)
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: third, Action: "join", WorkspaceID: shared.WorkspaceID}); err != nil {
		t.Fatal(err)
	}
	a := workspaceEvent(t, h, "parent", "parent-agent", "shared.ts")
	b := workspaceEvent(t, h, "child", "subagent", "shared-two.ts")
	got := h.SessionSnapshot("child")
	if len(got.Events) != 2 || got.Events[0].AgentID != a.AgentID || got.Events[1].AgentID != b.AgentID {
		t.Fatal("shared work lost authorship")
	}
	members := h.Workspaces()
	found := false
	for _, room := range members {
		if room.WorkspaceID == shared.WorkspaceID {
			found = reflect.DeepEqual(room.Members, []string{"child", "other-agent", "parent"})
		}
	}
	if !found {
		t.Fatal("shared member list missing")
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "parent", Action: "leave"}); err != nil {
		t.Fatal(err)
	}
	restored := h.SessionSnapshot("parent")
	if len(restored.Events) != 1 || restored.Events[0].ID != private.ID {
		t.Fatal("private history did not resume")
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "child", Action: "leave"}); err != nil {
		t.Fatal(err)
	}
	if restored := h.SessionSnapshot("child"); len(restored.Events) != 1 || restored.Events[0].ID != childPrivate.ID {
		t.Fatal("subagent private history did not resume")
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "parent", Action: "join", WorkspaceID: shared.WorkspaceID}); err != nil {
		t.Fatal(err)
	}
	if len(h.SessionSnapshot("parent").Events) != 2 {
		t.Fatal("shared history did not resume")
	}
}

func TestObservedChangesRequireActualPathsAndDoNotLeakOrDuplicate(t *testing.T) {
	h, _ := New("repo", "")
	defer h.Close()
	old := workspaceObserve(t, h, "local", "old.ts", "file")
	register(t, h, "codex")
	register(t, h, "claude")
	file, err := h.Publish(telemetry.Event{AgentID: "filesystem", NodeName: "main.ts", Action: "modified", Summary: "File modified", Why: "Observed a file change; no agent rationale was supplied.", Status: "ready", Source: "watcher", Kind: "file", Before: "before", After: "after", Diff: "exact diff"})
	if err != nil {
		t.Fatal(err)
	}
	if file.SessionID != "local" || len(h.SessionSnapshot("codex").Events) != 0 || len(h.SessionSnapshot("claude").Events) != 0 {
		t.Fatal("unattributed change leaked into private workspace")
	}
	if err := h.ClaimChanges("codex", []string{"old.ts", "main.ts"}); err != nil {
		t.Fatal(err)
	}
	got := h.SessionSnapshot("codex").Events
	if len(got) != 1 || got[0].ID == old.ID || got[0].Before != file.Before || got[0].After != file.After || got[0].Diff != file.Diff || got[0].Source != "watcher" {
		t.Fatal("capture was old, inexact, or invented author intent")
	}
	if err := h.ClaimChanges("codex", []string{"main.ts"}); err != nil {
		t.Fatal(err)
	}
	if err := h.ClaimChanges("claude", []string{"main.ts"}); err != nil {
		t.Fatal(err)
	}
	if len(h.SessionSnapshot("codex").Events) != 1 || len(h.SessionSnapshot("claude").Events) != 0 {
		t.Fatal("revision was duplicated across private workspaces")
	}
	pending, count := h.PendingChanges("codex", 50)
	if count != 1 {
		t.Fatal("owned exact revision not pending")
	}
	if _, err := h.ExplainChanges("claude", "claude", []telemetry.ChangeExplanation{{ChangeID: pending[0].ChangeID, Summary: "Explain change", Why: "This explanation belongs to a different workspace."}}); err == nil {
		t.Fatal("other workspace could explain a private change")
	}
	if _, err := h.ExplainChanges("codex", "codex", []telemetry.ChangeExplanation{{ChangeID: pending[0].ChangeID, Summary: "Explain change", Why: "Keep the precise revision connected to its actual decision."}}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceMembershipSettingsAndCaptureAssignmentsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	register(t, h, "parent")
	register(t, h, "child")
	private := workspaceEvent(t, h, "parent", "parent", "private.ts")
	shared, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "parent", Action: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "child", Action: "join", WorkspaceID: shared.WorkspaceID}); err != nil {
		t.Fatal(err)
	}
	workspaceObserve(t, h, "local", "shared.ts", "file")
	if err := h.ClaimChanges("child", []string{"shared.ts"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SetSessionUpdateMode("parent", telemetry.UpdatesMilestones); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if h.ResolveSession("parent") != shared.WorkspaceID || h.ResolveSession("child") != shared.WorkspaceID || h.SessionSnapshot("child").UpdateMode != telemetry.UpdatesMilestones {
		t.Fatal("shared routing or settings were lost")
	}
	if len(h.SessionSnapshot("child").Events) != 1 {
		t.Fatal("exact shared revision was lost")
	}
	if _, err := h.ConfigureWorkspace(WorkspaceInput{SessionID: "parent", Action: "leave"}); err != nil {
		t.Fatal(err)
	}
	if got := h.SessionSnapshot("parent").Events; len(got) != 1 || got[0].ID != private.ID {
		t.Fatal("private history was lost on restart")
	}
	if err := h.ClaimChanges("parent", []string{"shared.ts"}); err != nil {
		t.Fatal(err)
	}
	if len(h.SessionSnapshot("parent").Events) != 1 {
		t.Fatal("assigned shared revision was imported into private history")
	}
}

func TestWorkspaceJoinValidationAndIndependentActivation(t *testing.T) {
	h, _ := New("repo", "")
	defer h.Close()
	register(t, h, "one")
	view, stop := h.SubscribeSession("one")
	defer stop()
	<-view
	register(t, h, "two")
	drainWorkspaceStream(view)
	if _, err := h.ActivateSession("two"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-view:
		t.Fatalf("another activation took over pinned view: %+v", msg)
	default:
	}
	for _, input := range []WorkspaceInput{{SessionID: "one", Action: "join", WorkspaceID: "two"}, {SessionID: "one", Action: "join", WorkspaceID: "missing"}, {SessionID: "one", Action: "create", WorkspaceID: "two"}, {SessionID: "one", Action: "bad"}} {
		if _, err := h.ConfigureWorkspace(input); err == nil {
			t.Fatalf("accepted invalid sharing input: %+v", input)
		}
	}
}

func workspaceObserve(t *testing.T, h *Hub, session, node, kind string) telemetry.Event {
	t.Helper()
	event, err := h.Publish(telemetry.Event{SessionID: session, AgentID: "filesystem", NodeName: node, Kind: kind, Action: "modified", Summary: "File modified", Why: "A saved file changed.", Status: "ready", Source: "watcher"})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestLegacyChatRegistrationRetainsVisibleSavedHistory(t *testing.T) {
	h, _ := New("repo", "")
	defer h.Close()
	prior := workspaceEvent(t, h, "existing-chat", "agent", "saved.ts")
	register(t, h, "existing-chat")
	snapshot := h.SessionSnapshot("existing-chat")
	if len(snapshot.Events) != 1 || snapshot.Events[0].ID != prior.ID || snapshot.UpdatesFrom != "0" {
		t.Fatal("private workspace registration hid resumed saved history")
	}
}

func TestPartialCaptureRecordDoesNotClaimObservationOnRecovery(t *testing.T) {
	for _, part := range []string{"metadata", "revision"} {
		t.Run(part, func(t *testing.T) { testPartialCaptureRecovery(t, part) })
	}
}
func testPartialCaptureRecovery(t *testing.T, part string) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	register(t, h, "agent")
	source := workspaceObserve(t, h, "local", "file.ts", "file")
	if err := h.ClaimChanges("agent", []string{"file.ts"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(raw, []byte{10})
	captureAt := -1
	for i, line := range lines {
		var record sessionControl
		if json.Unmarshal(line, &record) == nil && record.Control == "capture" {
			captureAt = i
			break
		}
	}
	if captureAt < 0 {
		t.Fatal("capture record missing")
	}
	cut := len(lines[captureAt]) / 4
	if part == "revision" {
		cut = len(lines[captureAt]) * 3 / 4
	}
	partial := bytes.Join(lines[:captureAt], []byte{10})
	partial = append(partial, 10)
	partial = append(partial, lines[captureAt][:cut]...)
	if err := os.WriteFile(path, partial, 0600); err != nil {
		t.Fatal(err)
	}
	h, err = New("repo", path)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if h.assigned[source.ID] != "" || len(h.SessionSnapshot("agent").Events) != 0 {
		t.Fatal("partial capture claimed or invented a revision")
	}
	if err := h.ClaimChanges("agent", []string{"file.ts"}); err != nil {
		t.Fatal(err)
	}
	if len(h.SessionSnapshot("agent").Events) != 1 {
		t.Fatal("unclaimed exact revision could not be recovered")
	}
}
