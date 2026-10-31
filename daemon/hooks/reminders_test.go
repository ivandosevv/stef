package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

type reminderRequest struct {
	SessionID string   `json:"session_id"`
	Paths     []string `json:"paths"`
	SettleMS  int      `json:"settle_ms"`
}
type reminderFixture struct {
	root     string
	server   *httptest.Server
	requests []string
	bodies   []string
	input    reminderRequest
	activity telemetry.Activity
	mode     string
	from     string
	pending  []telemetry.PendingChange
}

func newReminderFixture(t *testing.T) *reminderFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &reminderFixture{root: root, mode: "files", from: "10"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		f.bodies = append(f.bodies, string(body))
		switch r.URL.Path {
		case "/healthz":
			json.NewEncoder(w).Encode(map[string]string{"name": "stef", "status": "ok", "version": "0.3.0", "repo": root})
		case "/api/activity":
			if err := json.Unmarshal(body, &f.activity); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusCreated)
		case "/api/explanation-reminder":
			if err := json.Unmarshal(body, &f.input); err != nil {
				t.Error(err)
			}
			json.NewEncoder(w).Encode(map[string]any{"repo": root, "session_id": f.activity.SessionID, "update_mode": f.mode, "updates_from": f.from, "pending_changes": f.pending, "pending_count": len(f.pending), "feedback": []string{"private-feedback"}, "code": "private-code-response"})
		default:
			t.Errorf("unexpected hook route: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func reminderPayload(t *testing.T, root, event, tool string, input any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"cwd": root, "session_id": "private-session", "hook_event_name": event, "tool_name": tool, "tool_input": input, "tool_response": map[string]string{"output": "private-output"}, "transcript_path": "private-transcript", "prompt": "private-prompt"})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func runReminder(t *testing.T, f *reminderFixture, client, event, tool string, input any) string {
	t.Helper()
	var output bytes.Buffer
	if err := Run(context.Background(), f.server.URL, client, bytes.NewReader(reminderPayload(t, f.root, event, tool, input)), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}
func contextText(t *testing.T, output string) string {
	t.Helper()
	var result struct {
		Output struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.Output.Event != "PostToolUse" {
		t.Fatal("incorrect event")
	}
	return result.Output.Context
}
func TestRunReminderUsesExactMultiFileRevisionsWithoutPrivateInput(t *testing.T) {
	f := newReminderFixture(t)
	f.pending = []telemetry.PendingChange{{ChangeID: "10", NodeName: "src/first.go", Action: "modified"}, {ChangeID: "12", NodeName: "new.go", Action: "created"}, {ChangeID: "13", NodeName: "old.go", Action: "deleted"}, {ChangeID: "14", NodeName: "renamed.go", Action: "created"}, {ChangeID: "15", NodeName: "unrelated.go", Action: "modified"}, {ChangeID: "9", NodeName: "src/first.go", Action: "modified"}, {ChangeID: "010", NodeName: "src/first.go", Action: "modified"}, {ChangeID: "16", NodeName: "new.go#symbol", Action: "modified"}, {ChangeID: "17", NodeName: "new.go", Action: "tested"}}
	patch := "*** Begin Patch\n*** Update File: src/first.go\n-private-source\n+private-source-new\n*** Add File: new.go\n+private-code\n*** Delete File: old.go\n*** Update File: old.go\n*** Move to: renamed.go\n*** End Patch"
	output := runReminder(t, f, "codex", "PostToolUse", "apply_patch", map[string]string{"command": patch})
	context := contextText(t, output)
	if strings.Join(f.requests, ",") != "/healthz,/api/activity,/api/explanation-reminder" {
		t.Fatalf("unexpected requests: %v", f.requests)
	}
	if strings.Join(f.input.Paths, ",") != "src/first.go,new.go,old.go,renamed.go" || f.input.SettleMS != 300 || f.input.SessionID != f.activity.SessionID {
		t.Fatalf("wrong metadata: %#v", f.input)
	}
	if !strings.Contains(context, "--report-changes --repo") || !strings.Contains(context, "before another edit, test, or final reply") {
		t.Fatal("missing instruction")
	}
	for _, want := range []string{`"change_id":"10"`, `"change_id":"12"`, `"change_id":"13"`, `"change_id":"14"`} {
		if !strings.Contains(context, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, secret := range []string{"private-", "unrelated.go", "*** Begin Patch", `"change_id":"9"`, `"change_id":"010"`, `"change_id":"16"`, `"change_id":"17"`, "permissionDecision", `"decision":`, `"continue":`} {
		if strings.Contains(output+strings.Join(f.bodies, ""), secret) {
			t.Errorf("unsafe material: %s", secret)
		}
	}
}
func TestRunReminderRecognizesSupportedEditInputs(t *testing.T) {
	for i, tc := range []struct {
		client, tool string
		input        any
	}{
		{"codex", "apply_patch", "*** Begin Patch\n*** Add File: main.go\n+content\n*** End Patch"},
		{"codex", "apply_patch", map[string]string{"patch": "*** Update File: main.go"}},
		{"codex", "apply_patch", map[string]string{"path": "main.go"}},
		{"claude", "Edit", map[string]string{"file_path": "main.go", "old_string": "private-old", "new_string": "private-new"}},
		{"claude", "Write", map[string]string{"file_path": "main.go", "content": "private-code"}},
		{"claude", "MultiEdit", map[string]any{"file_path": "main.go", "edits": []string{"private-edit"}}},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			f := newReminderFixture(t)
			f.pending = []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}
			output := runReminder(t, f, tc.client, "PostToolUse", tc.tool, tc.input)
			if len(f.input.Paths) != 1 || f.input.Paths[0] != "main.go" || output == "" {
				t.Fatalf("edit not reminded: %s %#v", output, f.input)
			}
			if strings.Contains(output+strings.Join(f.bodies, ""), "private-") {
				t.Fatal("private edit leaked")
			}
		})
	}
}
func TestRunReminderSkipsNonEditsAndSelfTools(t *testing.T) {
	for _, client := range []string{"codex", "claude"} {
		for i, tc := range []struct {
			event, tool string
			input       any
		}{
			{"PreToolUse", "Edit", map[string]string{"file_path": "main.go"}},
			{"Stop", "", nil},
			{"PostToolUse", "Read", map[string]string{"file_path": "main.go"}},
			{"PostToolUse", "Bash", map[string]string{"command": "go test ./...", "file_path": "main.go"}},
			{"PostToolUse", "Bash", map[string]string{"command": "echo changed > main.go"}},
			{"PostToolUse", "mcp__external__edit", map[string]string{"file_path": "main.go"}},
			{"PostToolUse", "mcp__stef__report_changes", nil},
			{"PostToolUse", "mcp__stef__set_workspace", nil},
			{"PostToolUse", "apply_patch", nil},
		} {
			t.Run(client+fmt.Sprint(i), func(t *testing.T) {
				f := newReminderFixture(t)
				f.pending = []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}
				if output := runReminder(t, f, client, tc.event, tc.tool, tc.input); output != "" || len(f.input.Paths) > 0 {
					t.Fatalf("irrelevant tool reminded: %s", output)
				}
				if tc.tool == "mcp__stef__report_changes" && len(f.requests) != 1 {
					t.Fatal("own MCP hook forwarded")
				}
			})
		}
	}
}

func TestSessionStartReportsOnlyTrustedWorkerIdentity(t *testing.T) {
	for _, client := range []string{"claude", "codex"} {
		t.Run(client, func(t *testing.T) {
			f := newReminderFixture(t)
			output := runReminder(t, f, client, "SessionStart", "", nil)
			var result struct {
				Output struct {
					Event   string `json:"hookEventName"`
					Context string `json:"additionalContext"`
				} `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil {
				t.Fatal(err)
			}
			if result.Output.Event != "SessionStart" || !strings.Contains(result.Output.Context, f.activity.SessionID) || !strings.Contains(result.Output.Context, f.activity.AgentID) {
				t.Fatal("startup did not expose its actual trusted worker identity")
			}
			if strings.Join(f.requests, ",") != "/healthz,/api/activity" || strings.Contains(output+strings.Join(f.bodies, ""), "private-") {
				t.Fatal("startup fetched pending data or exposed private input")
			}
		})
	}
}
func TestRunReminderHonorsModeAndClearedWindows(t *testing.T) {
	for i, tc := range []struct {
		mode, from string
		pending    []telemetry.PendingChange
	}{
		{"milestones", "10", []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}},
		{"disabled", "10", []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}},
		{"files", "12", []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}},
		{"activity", "12", []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}},
		{"files", "10", nil},
		{"activity", "10", nil},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			f := newReminderFixture(t)
			f.mode, f.from, f.pending = tc.mode, tc.from, tc.pending
			if output := runReminder(t, f, "codex", "PostToolUse", "apply_patch", map[string]string{"path": "main.go"}); output != "" {
				t.Fatalf("inactive revision reminded: %s", output)
			}
		})
	}
}

func TestRunReminderExplainsExactRevisionsInAllActivity(t *testing.T) {
	for _, client := range []string{"claude", "codex"} {
		t.Run(client, func(t *testing.T) {
			f := newReminderFixture(t)
			f.mode = "activity"
			f.pending = []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}
			tool := "Edit"
			input := map[string]string{"file_path": "main.go"}
			if client == "codex" {
				tool = "apply_patch"
				input = map[string]string{"path": "main.go"}
			}
			context := contextText(t, runReminder(t, f, client, "PostToolUse", tool, input))
			if !strings.Contains(context, "(All activity)") || strings.Contains(context, "(Every file)") {
				t.Fatal("All activity reminder displayed the wrong reporting mode")
			}
			if !strings.Contains(context, `"change_id":"11"`) || !strings.Contains(context, "before another edit, test, or final reply") {
				t.Fatal("All activity lost its exact file explanation reminder")
			}
			if strings.Contains(context, "private-") || strings.Join(f.requests, ",") != "/healthz,/api/activity,/api/explanation-reminder" {
				t.Fatal("All activity made extra requests or leaked private hook material")
			}
		})
	}
}
func TestRunReminderConfinesNestedPathsAndSymlinkEscapes(t *testing.T) {
	f := newReminderFixture(t)
	if err := os.Mkdir(filepath.Join(f.root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.WriteFile(filepath.Join(outside, "secret.go"), []byte("private-code"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.root, "escape")); err != nil {
		t.Fatal(err)
	}
	patch := "*** Update File: ../escape/secret.go\n*** Delete File: ../escape/missing.go\n*** Update File: ../../outside.go\n*** Update File: actual.go"
	paths, _ := reminderPaths("codex", reminderPayload(t, filepath.Join(f.root, "src"), "PostToolUse", "apply_patch", map[string]string{"command": patch}), f.root)
	if strings.Join(paths, ",") != "src/actual.go" {
		t.Fatalf("outside or symlink paths accepted: %v", paths)
	}
	raw := reminderPayload(t, outside, "PostToolUse", "apply_patch", map[string]string{"path": "secret.go"})
	var output bytes.Buffer
	if err := Run(context.Background(), f.server.URL, "codex", bytes.NewReader(raw), &output); err == nil || output.Len() != 0 || len(f.requests) != 1 {
		t.Fatal("outside cwd sent data")
	}
}
func TestRunReminderBoundsPathsAndModelContext(t *testing.T) {
	f := newReminderFixture(t)
	var patch strings.Builder
	for i := 0; i < 60; i++ {
		file := fmt.Sprintf("src/%s%d.go", strings.Repeat("long-path-", 35), i)
		fmt.Fprintf(&patch, "*** Update File: %s\n", file)
		f.pending = append(f.pending, telemetry.PendingChange{ChangeID: fmt.Sprint(100 + i), NodeName: file, Action: "modified"})
	}
	output := runReminder(t, f, "codex", "PostToolUse", "apply_patch", map[string]string{"command": patch.String()})
	context := contextText(t, output)
	if len(f.input.Paths) != 50 || len(context) > maxReminderBytes || !strings.Contains(context, `"change_id":"100"`) {
		t.Fatalf("unbounded hint: paths=%d context=%d", len(f.input.Paths), len(context))
	}
	if strings.Contains(context, `"change_id":"159"`) {
		t.Fatal("unrequested revision included")
	}
	if !strings.Contains(context, `"incomplete":true`) || !strings.Contains(context, `"edited_paths_omitted":10`) || !strings.Contains(context, "/api/explanation-reminder") {
		t.Fatal("bounded reminder hid omitted revisions or edit paths")
	}
}

func TestRunReminderDisclosesRevisionLimitAndCompleteHints(t *testing.T) {
	for _, count := range []int{1, 70} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f := newReminderFixture(t)
			for i := 0; i < count; i++ {
				f.pending = append(f.pending, telemetry.PendingChange{ChangeID: fmt.Sprint(100 + i), NodeName: "main.go", Action: "modified"})
			}
			context := contextText(t, runReminder(t, f, "codex", "PostToolUse", "apply_patch", map[string]string{"path": "main.go"}))
			var metadata struct {
				Pending    []telemetry.PendingChange `json:"pending_changes"`
				Count      int                       `json:"pending_count"`
				Omitted    int                       `json:"omitted_revisions"`
				Incomplete bool                      `json:"incomplete"`
			}
			_, raw, found := strings.Cut(context, " Metadata: ")
			if !found || json.Unmarshal([]byte(raw), &metadata) != nil || metadata.Count != count || metadata.Omitted != count-len(metadata.Pending) || metadata.Incomplete != (count > 50) {
				t.Fatal("reminder did not disclose its exact coverage")
			}
		})
	}
}

func TestRunReminderDisclosesOmittedPathsEvenWhenSelectedPathsExplained(t *testing.T) {
	f := newReminderFixture(t)
	var patch strings.Builder
	for i := 0; i < 51; i++ {
		fmt.Fprintf(&patch, "*** Update File: file%d.go\n", i)
	}
	context := contextText(t, runReminder(t, f, "codex", "PostToolUse", "apply_patch", map[string]string{"command": patch.String()}))
	if !strings.Contains(context, `"edited_paths_omitted":1`) || !strings.Contains(context, `"incomplete":true`) || !strings.Contains(context, "Use the path-filtered recovery") {
		t.Fatal("omitted unqueried edit path was treated as already explained")
	}
}
func TestRunReminderRejectsUnhealthyCompanion(t *testing.T) {
	for _, version := range []string{"", "0.2.0"} {
		t.Run(version, func(t *testing.T) {
			root, _ := filepath.EvalSymlinks(t.TempDir())
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" {
					posts++
				}
				json.NewEncoder(w).Encode(map[string]string{"repo": root, "name": "stef", "status": "ok", "version": version})
			}))
			defer server.Close()
			var output bytes.Buffer
			if err := Run(context.Background(), server.URL, "codex", bytes.NewReader(reminderPayload(t, root, "PostToolUse", "apply_patch", map[string]string{"path": "main.go"})), &output); err == nil || posts != 0 || output.Len() != 0 {
				t.Fatal("unverified version accepted")
			}
		})
	}
}
func TestRunReminderCancellationAndTimeoutProduceNoHint(t *testing.T) {
	f := newReminderFixture(t)
	var output bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw := reminderPayload(t, f.root, "PostToolUse", "apply_patch", map[string]string{"path": "main.go"})
	if err := Run(ctx, f.server.URL, "codex", bytes.NewReader(raw), &output); err == nil || len(f.requests) != 0 || output.Len() != 0 {
		t.Fatal("canceled hook sent data")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(100 * time.Millisecond) }))
	defer server.Close()
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := Run(ctx, server.URL, "codex", bytes.NewReader(raw), &output); err == nil || output.Len() != 0 || time.Since(start) > 90*time.Millisecond {
		t.Fatal("timeout did not fail open")
	}
}
func TestSendRemainsObservationOnly(t *testing.T) {
	f := newReminderFixture(t)
	f.pending = []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}
	if err := Send(context.Background(), f.server.URL, "codex", bytes.NewReader(reminderPayload(t, f.root, "PostToolUse", "apply_patch", map[string]string{"path": "main.go"}))); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.requests, ",") != "/healthz,/api/activity" {
		t.Fatalf("observation-only API changed: %v", f.requests)
	}
}

func TestRunReminderIgnoresFailedEditsAndUnsupportedClients(t *testing.T) {
	t.Run("failed-edit", func(t *testing.T) {
		f := newReminderFixture(t)
		var output bytes.Buffer
		raw := reminderPayload(t, f.root, "PostToolUse", "apply_patch", map[string]string{"path": "main.go"})
		var payload map[string]any
		json.Unmarshal(raw, &payload)
		payload["tool_response"] = map[string]any{"isError": true}
		raw, _ = json.Marshal(payload)
		if err := Run(context.Background(), f.server.URL, "codex", bytes.NewReader(raw), &output); err != nil {
			t.Fatal(err)
		}
		if output.Len() != 0 || strings.Join(f.requests, ",") != "/healthz,/api/activity" {
			t.Fatal("failed edit reminded")
		}
	})
	t.Run("cursor", func(t *testing.T) {
		f := newReminderFixture(t)
		var output bytes.Buffer
		raw, _ := json.Marshal(map[string]any{"cwd": f.root, "conversation_id": "private-chat", "workspace_roots": []string{f.root}, "hook_event_name": "postToolUse", "tool_name": "Edit", "tool_input": map[string]string{"file_path": "main.go"}})
		if err := Run(context.Background(), f.server.URL, "cursor", bytes.NewReader(raw), &output); err != nil {
			t.Fatal(err)
		}
		if output.Len() != 0 || strings.Join(f.requests, ",") != "/healthz,/api/activity" {
			t.Fatal("unsupported client reminder")
		}
	})
	t.Run("missing-cwd", func(t *testing.T) {
		f := newReminderFixture(t)
		var output bytes.Buffer
		raw, _ := json.Marshal(map[string]any{"session_id": "private-chat", "hook_event_name": "PostToolUse", "tool_name": "Edit", "tool_input": map[string]string{"file_path": "main.go"}})
		if err := Run(context.Background(), f.server.URL, "claude", bytes.NewReader(raw), &output); err != nil {
			t.Fatal(err)
		}
		if output.Len() != 0 || strings.Join(f.requests, ",") != "/healthz,/api/activity" {
			t.Fatal("unverified cwd reminder")
		}
	})
}
func TestRunReminderRejectsBrokenOrMismatchedMetadata(t *testing.T) {
	for _, broken := range []string{"http-error", "bad-json", "root", "session", "window"} {
		t.Run(broken, func(t *testing.T) {
			root, _ := filepath.EvalSymlinks(t.TempDir())
			var activity telemetry.Activity
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/healthz":
					json.NewEncoder(w).Encode(map[string]string{"repo": root, "name": "stef", "status": "ok", "version": "0.3.0"})
				case "/api/activity":
					json.NewDecoder(r.Body).Decode(&activity)
					w.WriteHeader(http.StatusCreated)
				case "/api/explanation-reminder":
					if broken == "http-error" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					if broken == "bad-json" {
						io.WriteString(w, "not json")
						return
					}
					state := map[string]any{"repo": root, "session_id": activity.SessionID, "update_mode": "files", "updates_from": "10", "pending_changes": []telemetry.PendingChange{{ChangeID: "11", NodeName: "main.go", Action: "modified"}}}
					switch broken {
					case "root":
						state["repo"] = "/outside/not-stef"
					case "session":
						state["session_id"] = "another-chat"
					case "window":
						state["updates_from"] = "invalid"
					}
					json.NewEncoder(w).Encode(state)
				default:
					t.Errorf("unexpected route: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			var output bytes.Buffer
			if err := Run(context.Background(), server.URL, "codex", bytes.NewReader(reminderPayload(t, root, "PostToolUse", "apply_patch", map[string]string{"path": "main.go"})), &output); err == nil || output.Len() != 0 {
				t.Fatal("broken metadata was reminded")
			}
		})
	}
}
