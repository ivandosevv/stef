package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hooks"
	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestCappedHookRecoveryExplainsOwnFilesBehindUnrelatedBacklog(t *testing.T) {
	h, server := setup(t)
	root, err := filepath.EvalSymlinks(h.Snapshot().Repo)
	if err != nil {
		t.Fatal(err)
	}
	var patch strings.Builder
	paths := make([]string, 30)
	for i := range paths {
		paths[i] = strings.Repeat("deep/", 60) + fmt.Sprintf("file%d.go", i)
		fmt.Fprintf(&patch, "*** Update File: %s\n", paths[i])
	}
	payload, _ := json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "session_id": "recovery-chat", "cwd": root, "tool_name": "apply_patch", "tool_input": map[string]string{"command": patch.String()}})
	activity, err := hooks.Codex(payload, root)
	if err != nil {
		t.Fatal(err)
	}
	observe := func(node string) {
		t.Helper()
		if _, err := h.Publish(telemetry.Event{SessionID: activity.SessionID, AgentID: "filesystem", NodeName: node, Action: "modified", Summary: "File saved", Why: "A file save was observed.", Source: "watcher", Kind: "file"}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 60; i++ {
		observe(fmt.Sprintf("unrelated%d.go", i))
	}
	for _, path := range paths {
		observe(path)
	}
	var hookOutput bytes.Buffer
	if err := hooks.Run(context.Background(), server.URL, "codex", bytes.NewReader(payload), &hookOutput); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Output struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(hookOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	_, raw, found := strings.Cut(envelope.Output.Context, " Metadata: ")
	var hint struct {
		Pending    []telemetry.PendingChange `json:"pending_changes"`
		Omitted    int                       `json:"omitted_revisions"`
		Incomplete bool                      `json:"incomplete"`
	}
	if !found || json.Unmarshal([]byte(raw), &hint) != nil || !hint.Incomplete || hint.Omitted == 0 || !strings.Contains(envelope.Output.Context, server.URL+"/api/explanation-reminder") {
		t.Fatal("long-path reminder did not disclose a usable filtered recovery")
	}
	post := func(path string, input, output any) {
		t.Helper()
		body, _ := json.Marshal(input)
		response, err := http.Post(server.URL+path, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 && response.StatusCode != 201 {
			t.Fatalf("%s: HTTP %d", path, response.StatusCode)
		}
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			t.Fatal(err)
		}
	}
	report := func(pending []telemetry.PendingChange) bridge.ChangesReceipt {
		t.Helper()
		changes := make([]telemetry.ChangeExplanation, len(pending))
		for i, file := range pending {
			changes[i] = telemetry.ChangeExplanation{ChangeID: file.ChangeID, Summary: "Verify recovery", Why: "Keep every edited revision explained during bounded recovery."}
		}
		var receipt bridge.ChangesReceipt
		post("/api/changes", bridge.ChangesInput{SessionID: activity.SessionID, AgentID: "editing-agent", Changes: changes}, &receipt)
		if !receipt.Accepted {
			t.Fatal("own exact batch was not accepted")
		}
		return receipt
	}
	receipt := report(hint.Pending)
	if len(receipt.PendingChanges) != 50 {
		t.Fatal("fixture does not exercise global pending truncation")
	}
	for _, pending := range receipt.PendingChanges {
		if !strings.HasPrefix(pending.NodeName, "unrelated") {
			t.Fatal("global receipt did not remain crowded by unrelated saves")
		}
	}
	var remaining telemetry.ReminderState
	post("/api/explanation-reminder", telemetry.ReminderRequest{SessionID: activity.SessionID, Paths: paths}, &remaining)
	if remaining.PendingCount != hint.Omitted || len(remaining.PendingChanges) != hint.Omitted {
		t.Fatal("filtered discovery lost omitted own revisions")
	}
	for _, pending := range remaining.PendingChanges {
		if !strings.HasPrefix(pending.NodeName, "deep/") {
			t.Fatal("recovery included unrelated author intent")
		}
	}
	report(remaining.PendingChanges)
	post("/api/explanation-reminder", telemetry.ReminderRequest{SessionID: activity.SessionID, Paths: paths}, &remaining)
	if remaining.PendingCount != 0 {
		t.Fatal("some own edited revisions remain unexplained")
	}
	_, unrelated := h.PendingChanges(activity.SessionID, 50)
	if unrelated != 60 {
		t.Fatal("recovery falsely explained unrelated files")
	}
}

func TestExplanationReminderHTTPReturnsOnlyMatchingMetadata(t *testing.T) {
	h, server := setup(t)
	observe := func(node string) telemetry.Event {
		t.Helper()
		stored, err := h.Publish(telemetry.Event{SessionID: "chat-a", AgentID: "filesystem", NodeName: node, Action: "modified", Summary: "File saved", Why: "A saved file changed.", Status: "ready", Source: "watcher", Kind: "file", Before: "private old code", After: "private new code", Diff: "private diff"})
		if err != nil {
			t.Fatal(err)
		}
		return stored
	}
	for i := 0; i < 60; i++ {
		observe(fmt.Sprintf("other-%d.go", i))
	}
	file := observe("main.go")
	note, _ := (telemetry.Feedback{SessionID: "chat-a", TargetAgent: "agent", Kind: "question", Text: "private developer note"}).Event()
	note, err := h.Publish(note)
	if err != nil {
		t.Fatal(err)
	}
	before := h.SessionSnapshot("chat-a")
	input, _ := json.Marshal(telemetry.ReminderRequest{SessionID: "chat-a", Paths: []string{"main.go"}})
	response, err := http.Post(server.URL+"/api/explanation-reminder", "application/json", bytes.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var state telemetry.ReminderState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || state.Repo != before.Repo || state.SessionID != "chat-a" || state.UpdateMode != telemetry.UpdatesFiles || state.UpdatesFrom != before.UpdatesFrom || state.PendingCount != 1 || len(state.PendingChanges) != 1 || state.PendingChanges[0].ChangeID != file.ID {
		t.Fatalf("incorrect HTTP reminder: %d %+v", response.StatusCode, state)
	}
	for _, forbidden := range []string{"private", "before", "after", "diff", "feedback", "events", "summary", "why"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("HTTP reminder leaked %s", forbidden)
		}
	}
	if len(h.SessionSnapshot("chat-a").Events) != len(before.Events) || h.Snapshot().SessionID != "local" {
		t.Fatal("HTTP reminder altered saved workflow or chat selection")
	}
	if notes := h.OfferFeedback("agent", "chat-a"); len(notes) != 1 || notes[0].ID != note.ID {
		t.Fatal("HTTP reminder consumed the developer note queue")
	}
}

func TestExplanationReminderHTTPRejectsMalformedAndUnboundedRequests(t *testing.T) {
	h, server := setup(t)
	for _, tc := range []struct {
		name, media, body string
		status            int
	}{
		{"wrong content type", "text/plain", `{"paths":["main.go"]}`, 415},
		{"missing files", "application/json", `{}`, 400},
		{"null body", "application/json", `null`, 400},
		{"symbol", "application/json", `{"paths":["main.go#Run"]}`, 400},
		{"traversal", "application/json", `{"paths":["../main.go"]}`, 400},
		{"invalid chat", "application/json", `{"paths":["main.go"],"session_id":" chat"}`, 400},
		{"negative wait", "application/json", `{"paths":["main.go"],"settle_ms":-1}`, 400},
		{"excessive wait", "application/json", `{"paths":["main.go"],"settle_ms":1001}`, 400},
		{"ignored field", "application/json", `{"paths":["main.go"],"agent_id":"agent"}`, 400},
		{"trailing object", "application/json", `{"paths":["main.go"]}{}`, 400},
		{"oversize JSON", "application/json", `{"paths":["` + strings.Repeat("a", 33*1024) + `"]}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := http.Post(server.URL+"/api/explanation-reminder", tc.media, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("got %d, want %d", response.StatusCode, tc.status)
			}
		})
	}
	if len(h.Snapshot().Events) != 0 {
		t.Fatal("invalid requests produced workflow events")
	}
}

type reminderBody struct {
	io.Reader
	done chan struct{}
	once sync.Once
}

func (b *reminderBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.once.Do(func() { close(b.done) })
	}
	return n, err
}

func (*reminderBody) Close() error { return nil }

func TestExplanationReminderReturnsLatestModeAfterSettle(t *testing.T) {
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := h.Publish(telemetry.Event{AgentID: "filesystem", NodeName: "main.go", Action: "modified", Summary: "File saved", Why: "A file changed.", Source: "watcher", Kind: "file"}); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []telemetry.UpdateMode{telemetry.UpdatesMilestones, telemetry.UpdatesDisabled} {
		t.Run(string(mode), func(t *testing.T) {
			if _, err := h.SetUpdateMode(telemetry.UpdatesFiles); err != nil {
				t.Fatal(err)
			}
			body := &reminderBody{Reader: strings.NewReader(`{"paths":["main.go"],"settle_ms":200}`), done: make(chan struct{})}
			request := httptest.NewRequest(http.MethodPost, "http://localhost:4820/api/explanation-reminder", body)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				handler(h, t.TempDir(), "4820").ServeHTTP(response, request)
				close(done)
			}()
			select {
			case <-body.done:
			case <-time.After(time.Second):
				t.Fatal("request was not decoded")
			}
			if _, err := h.SetUpdateMode(mode); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("bounded reminder did not finish")
			}
			var state telemetry.ReminderState
			if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
			if response.Code != 200 || state.UpdateMode != mode || state.UpdatesFrom != h.Snapshot().UpdatesFrom || state.PendingCount != 0 || len(state.PendingChanges) != 0 {
				t.Fatalf("settle returned stale reporting preferences: %d %+v", response.Code, state)
			}
		})
	}
}

func TestExplanationReminderSettleCancelsPromptly(t *testing.T) {
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &reminderBody{Reader: strings.NewReader(`{"paths":["main.go"],"settle_ms":1000}`), done: make(chan struct{})}
	request := httptest.NewRequest(http.MethodPost, "http://localhost:4820/api/explanation-reminder", body).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	serve := handler(h, t.TempDir(), "4820")
	done := make(chan struct{})
	go func() {
		serve.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-body.done:
	case <-time.After(time.Second):
		t.Fatal("request was not decoded")
	}
	start := time.Now()
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("canceled reminder did not finish promptly")
	}
	if response.Code != 400 || time.Since(start) > 500*time.Millisecond || len(h.Snapshot().Events) != 0 {
		t.Fatal("canceled reminder waited or changed workflow")
	}
}
