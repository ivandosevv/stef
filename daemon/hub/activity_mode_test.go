package hub_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func choice(session string) telemetry.Event {
	return telemetry.Event{SessionID: session, AgentID: "agent", NodeName: "@workflow", Action: "decided", Summary: "Keep capture local", Why: "Observe supported actions without requesting another model response.", Status: "ready"}
}

func TestActivityRecordsSupportedObservationsAndChoicesOnlyWhileSelected(t *testing.T) {
	h := newHub(t)
	for _, mode := range []telemetry.UpdateMode{telemetry.UpdatesFiles, telemetry.UpdatesMilestones, telemetry.UpdatesDisabled} {
		setMode(t, h, mode)
		stored, err := h.Publish(choice("chat-a"))
		if err != nil || stored.ID != "" {
			t.Fatalf("%s recorded an extra choice: %+v %v", mode, stored, err)
		}
	}
	setMode(t, h, telemetry.UpdatesActivity)
	phases := []string{"started", "reading", "searching", "editing", "testing", "terminal", "tool_done", "completed", "failed"}
	for _, phase := range phases {
		for range 2 {
			activity, err := (telemetry.Activity{SessionID: "chat-a", AgentID: "host", Phase: phase, Tool: "Read"}).Event()
			if err != nil {
				t.Fatal(err)
			}
			stored, err := h.Publish(activity)
			if err != nil || stored.ID == "" || stored.RecordingMode != telemetry.UpdatesActivity {
				t.Fatalf("%s observation lost: %+v %v", phase, stored, err)
			}
		}
	}
	stored, err := h.Publish(choice("chat-a"))
	if err != nil || stored.ID == "" || len(h.SessionSnapshot("chat-a").Events) != 2*len(phases)+1 {
		t.Fatalf("distinct activity or decision lost: %+v %v", stored, err)
	}
	setMode(t, h, telemetry.UpdatesFiles)
	if reply, err := h.Publish(contextReply(stored)); err != nil || reply.ID == "" {
		t.Fatalf("explicit retained-choice question rejected outside activity: %+v %v", reply, err)
	}
}

func TestActivityFileReasonsAreExactAndFutureOnlyAcrossRestart(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	old := observe(t, h, "chat-a", "main.ts", "file")
	setMode(t, h, telemetry.UpdatesActivity)
	fresh := observe(t, h, "chat-a", "main.ts", "file")
	assertPending := func(h *hub.Hub) {
		t.Helper()
		state, err := h.ExplanationReminder("chat-a", []string{"main.ts"})
		if err != nil || state.UpdateMode != telemetry.UpdatesActivity || state.PendingCount != 1 || len(state.PendingChanges) != 1 || state.PendingChanges[0].ChangeID != fresh.ID {
			t.Fatalf("activity reminder backfilled or lost exact revision: %+v %v", state, err)
		}
		pending, count := h.PendingChanges("chat-a", 50)
		if count != 1 || len(pending) != 1 || pending[0].ChangeID != fresh.ID {
			t.Fatalf("activity pending coverage differs: %+v %d", pending, count)
		}
	}
	assertPending(h)
	if _, err := h.ExplainChanges("chat-a", "agent", []telemetry.ChangeExplanation{explanation(fresh.ID), explanation(old.ID)}); err == nil {
		t.Fatal("activity accepted historical automatic explanations")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	assertPending(h)
	explained, err := h.ExplainChanges("chat-a", "agent", []telemetry.ChangeExplanation{explanation(fresh.ID)})
	if err != nil || len(explained) != 1 || explained[0].RecordingMode != telemetry.UpdatesActivity || explained[0].RecordingFrom != fresh.ID {
		t.Fatalf("activity explanation lost its original window: %+v %v", explained, err)
	}
	if _, count := h.PendingChanges("chat-a", 50); count != 0 {
		t.Fatal("activity explanation left its exact revision pending")
	}
	setMode(t, h, telemetry.UpdatesFiles)
	if _, count := h.PendingChanges("chat-a", 50); count != 0 {
		t.Fatal("returning to files backfilled activity history")
	}
}

func TestRecordingProvenanceCannotBeSpoofedAndSurvivesWorkspaceRecovery(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.RegisterSession("private"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.RegisterSession("peer"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SetSessionUpdateMode("private", telemetry.UpdatesActivity); err != nil {
		t.Fatal(err)
	}
	privateChoice := choice("private")
	privateChoice.RecordingMode, privateChoice.RecordingFrom = telemetry.UpdatesFiles, "99999"
	privateChoice, err = h.Publish(privateChoice)
	if err != nil || privateChoice.RecordingMode != telemetry.UpdatesActivity || privateChoice.RecordingFrom != "1" {
		t.Fatalf("caller controlled activity provenance: %+v %v", privateChoice, err)
	}
	shared, err := h.ConfigureWorkspace(hub.WorkspaceInput{SessionID: "private", Action: "create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.ConfigureWorkspace(hub.WorkspaceInput{SessionID: "peer", Action: "join", WorkspaceID: shared.WorkspaceID}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.SetSessionUpdateMode("peer", telemetry.UpdatesActivity); err != nil {
		t.Fatal(err)
	}
	neutral := observe(t, h, "", "main.ts", "file")
	if err := h.ClaimChanges("peer", []string{"main.ts"}); err != nil {
		t.Fatal(err)
	}
	captured := h.SessionSnapshot("peer").Events
	if len(captured) != 1 || captured[0].SessionID != shared.WorkspaceID || captured[0].RecordingMode != telemetry.UpdatesActivity || captured[0].RecordingFrom != "2" {
		t.Fatalf("claimed revision did not use actual shared mode/window: %+v", captured)
	}
	if err := h.ClaimChanges("private", []string{"main.ts"}); err != nil || len(h.SessionSnapshot("peer").Events) != 1 {
		t.Fatal("same observed revision was copied twice into shared history", err)
	}
	if _, err := h.ConfigureWorkspace(hub.WorkspaceInput{SessionID: "private", Action: "leave"}); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	private := h.SessionSnapshot("private")
	room := h.SessionSnapshot("peer")
	if len(private.Events) != 1 || private.Events[0].ID != privateChoice.ID || private.Events[0].RecordingMode != telemetry.UpdatesActivity || len(room.Events) != 1 || room.Events[0].RecordingMode != telemetry.UpdatesActivity || room.Events[0].ID == neutral.ID {
		t.Fatalf("restart changed private/shared provenance: private=%+v shared=%+v", private, room)
	}
	if err := h.ClaimChanges("private", []string{"main.ts"}); err != nil || len(h.SessionSnapshot("private").Events) != 1 {
		t.Fatal("restart exposed another workspace's exact revision", err)
	}
}

func TestActivityObservationContextUsesOriginalVisibility(t *testing.T) {
	h := newHub(t)
	legacy, _ := (telemetry.Activity{SessionID: "chat-a", AgentID: "host", Phase: "terminal"}).Event()
	legacy.RecordingMode, legacy.RecordingFrom = telemetry.UpdatesActivity, "0"
	legacy, err := h.Publish(legacy)
	if err != nil || legacy.RecordingMode != telemetry.UpdatesFiles {
		t.Fatalf("caller promoted a hidden observation: %+v %v", legacy, err)
	}
	setMode(t, h, telemetry.UpdatesActivity)
	var retained []telemetry.Event
	for _, phase := range []string{"started", "reading", "searching", "editing", "testing", "terminal", "tool_done", "completed", "failed"} {
		e, _ := (telemetry.Activity{SessionID: "chat-a", AgentID: "host", Phase: phase}).Event()
		e, err = h.Publish(e)
		if err != nil {
			t.Fatal(err)
		}
		retained = append(retained, e)
	}
	setMode(t, h, telemetry.UpdatesFiles)
	if _, err := h.Publish(contextReply(legacy)); err == nil {
		t.Fatal("activity selection promoted an originally hidden files-mode tool")
	}
	for _, target := range retained {
		if reply, err := h.Publish(contextReply(target)); err != nil || reply.ID == "" {
			t.Fatalf("explicit saved activity context rejected: %+v %v", target, err)
		}
	}
	setMode(t, h, telemetry.UpdatesDisabled)
	if reply, err := h.Publish(contextReply(retained[0])); err != nil || reply.ID != "" {
		t.Fatalf("disabled activity context persisted: %+v %v", reply, err)
	}
}

func TestActivityCaptureUnionDoesNotDisableOtherWorkers(t *testing.T) {
	h := newHub(t)
	for _, id := range []string{"one", "two"} {
		if _, err := h.RegisterSession(id); err != nil {
			t.Fatal(err)
		}
	}
	_, initial := h.CaptureState()
	if _, err := h.SetSessionUpdateMode("one", telemetry.UpdatesActivity); err != nil {
		t.Fatal(err)
	}
	mode, generation := h.CaptureState()
	if mode != telemetry.UpdatesActivity || generation != initial {
		t.Fatalf("switching detailed modes reset passive capture: %s %d", mode, generation)
	}
	if _, err := h.SetSessionUpdateMode("two", telemetry.UpdatesDisabled); err != nil {
		t.Fatal(err)
	}
	if mode, _ := h.CaptureState(); mode != telemetry.UpdatesActivity {
		t.Fatal("disabled peer stopped activity capture")
	}
	if _, err := h.SetSessionUpdateMode("one", telemetry.UpdatesDisabled); err != nil {
		t.Fatal(err)
	}
	if mode, stopped := h.CaptureState(); mode != telemetry.UpdatesDisabled || stopped <= generation {
		t.Fatal("all disabled workers retained passive file capture")
	}
	if _, err := h.SetSessionUpdateMode("two", telemetry.UpdatesActivity); err != nil {
		t.Fatal(err)
	}
	if mode, resumed := h.CaptureState(); mode != telemetry.UpdatesActivity || resumed <= generation {
		t.Fatal("activity did not resume passive file capture")
	}
	if h.SessionSnapshot("one").UpdateMode != telemetry.UpdatesDisabled {
		t.Fatal("peer mode switch changed the private disabled workspace")
	}
	from, _ := strconv.ParseUint(h.SessionSnapshot("two").UpdatesFrom, 10, 64)
	if from != 1 {
		t.Fatal("mode changes without events consumed reporting IDs")
	}
}

func TestLegacyRecordingMetadataRecoversWithoutPromotingHiddenActivity(t *testing.T) {
	journal := filepath.Join(t.TempDir(), "events.jsonl")
	h, err := hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := (telemetry.Activity{SessionID: "chat-a", AgentID: "host", Phase: "terminal"}).Event()
	old, err = h.Publish(old)
	if err != nil {
		t.Fatal(err)
	}
	setMode(t, h, telemetry.UpdatesMilestones)
	milestone := event("agent", "feature", "ready")
	milestone.SessionID = "chat-a"
	milestone, err = h.Publish(milestone)
	if err != nil {
		t.Fatal(err)
	}
	setMode(t, h, telemetry.UpdatesActivity)
	unmarked, _ := (telemetry.Activity{SessionID: "chat-a", AgentID: "host", Phase: "terminal"}).Event()
	unmarked, err = h.Publish(unmarked)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	var legacy strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		delete(record, "recording_mode")
		delete(record, "recording_from")
		raw, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		legacy.Write(raw)
		legacy.WriteByte('\n')
	}
	if err := os.WriteFile(journal, []byte(legacy.String()), 0600); err != nil {
		t.Fatal(err)
	}
	h, err = hub.New("personal", journal)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	events := h.SessionSnapshot("chat-a").Events
	if len(events) != 3 || events[0].ID != old.ID || events[0].RecordingMode != telemetry.UpdatesFiles || events[0].RecordingFrom != "0" || events[1].ID != milestone.ID || events[1].RecordingMode != telemetry.UpdatesMilestones || events[1].RecordingFrom != milestone.ID || events[2].ID != unmarked.ID || events[2].RecordingMode != telemetry.UpdatesFiles {
		t.Fatalf("legacy recording provenance promoted unknown activity or lost original window: %+v", events)
	}
	for _, target := range []telemetry.Event{events[0], events[2]} {
		if _, err := h.Publish(contextReply(target)); err == nil {
			t.Fatal("missing original metadata promoted a hidden terminal observation")
		}
	}
}

func TestAutomaticSingleReasonsCannotCrossRecordingWindows(t *testing.T) {
	for _, mode := range []telemetry.UpdateMode{telemetry.UpdatesFiles, telemetry.UpdatesActivity} {
		t.Run(string(mode), func(t *testing.T) {
			h := newHub(t)
			old := observe(t, h, "chat-a", "main.ts", "file")
			setMode(t, h, telemetry.UpdatesMilestones)
			setMode(t, h, mode)
			automatic := event("agent", old.NodeName, "ready")
			automatic.SessionID, automatic.ChangeID = old.SessionID, old.ID
			if _, err := h.Publish(automatic); err == nil || !strings.Contains(err.Error(), "context_id") {
				t.Fatalf("%s allowed a direct old-window explanation or gave no recovery hint: %v", mode, err)
			}
			if len(h.SessionSnapshot("chat-a").Events) != 1 {
				t.Fatal("rejected automatic reason changed retained history")
			}
			if _, count := h.PendingChanges("chat-a", 50); count != 0 {
				t.Fatal("old revision was promoted into current pending reasons")
			}
			if reply, err := h.Publish(contextReply(old)); err != nil || reply.ID != "2" {
				t.Fatalf("retained explicit context was rejected or invalid report consumed an ID: %+v %v", reply, err)
			}
			fresh := observe(t, h, "chat-a", "main.ts", "file")
			automatic.ChangeID = fresh.ID
			if reply, err := h.Publish(automatic); err != nil || reply.ID == "" || reply.ChangeID != fresh.ID {
				t.Fatalf("exact current-window single reason rejected: %+v %v", reply, err)
			}
			if _, count := h.PendingChanges("chat-a", 50); count != 0 {
				t.Fatal("current exact revision was not explained")
			}
		})
	}
}
