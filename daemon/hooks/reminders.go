package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

const maxReminderBytes = 9000

var reminderPatchPath = regexp.MustCompile(`(?m)^\*\*\* (?:(?:Add|Update|Delete) File|Move to): (.+)$`)

// Run forwards a lifecycle observation and, for supported post-edit hooks,
// returns bounded model-visible context. It makes no model calls and never
// blocks, rewrites, or replaces the agent's tool result. Main fails open on errors.
func Run(ctx context.Context, endpoint, clientName string, input io.Reader, output io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	delivered, err := forward(ctx, endpoint, clientName, input, true)
	if err != nil || output == nil || delivered.activity.SessionID == "" {
		return err
	}
	if delivered.activity.Phase == "started" && (clientName == "claude" || clientName == "codex") {
		var input struct {
			Event string `json:"hook_event_name"`
		}
		if json.Unmarshal(delivered.raw, &input) != nil || (input.Event != "SessionStart" && input.Event != "UserPromptSubmit") {
			return nil
		}
		metadata, _ := json.Marshal(struct {
			SessionID string `json:"session_id"`
			AgentID   string `json:"agent_id"`
		}{delivered.activity.SessionID, delivered.activity.AgentID})
		return writeHookContext(output, input.Event, "stef worker identity: "+string(metadata)+". Bind this exact session_id with set_session, omitting agent_id there; use this session_id and agent_id for reports and check-ins. Each subagent needs its own worker identity. Share only through an explicit set_workspace join.")
	}
	if delivered.activity.Phase != "tool_done" {
		return nil
	}
	paths, omittedPaths := reminderPaths(clientName, delivered.raw, delivered.root)
	if len(paths) == 0 {
		return nil
	}
	data, err := json.Marshal(struct {
		SessionID string   `json:"session_id"`
		Paths     []string `json:"paths"`
		SettleMS  int      `json:"settle_ms"`
	}{delivered.activity.SessionID, paths, 300})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(endpoint, "/")+"/api/explanation-reminder", bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := delivered.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("explanation reminder unavailable (HTTP %d)", response.StatusCode)
	}
	var state struct {
		Repo           string                    `json:"repo"`
		SessionID      string                    `json:"session_id"`
		UpdateMode     telemetry.UpdateMode      `json:"update_mode"`
		UpdatesFrom    string                    `json:"updates_from"`
		PendingChanges []telemetry.PendingChange `json:"pending_changes"`
		PendingCount   int                       `json:"pending_count"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&state); err != nil {
		return fmt.Errorf("decode explanation reminder: %w", err)
	}
	root, err := filepath.EvalSymlinks(state.Repo)
	if err != nil || root != delivered.root || state.SessionID != delivered.activity.SessionID {
		return fmt.Errorf("explanation reminder root or chat does not match")
	}
	if !state.UpdateMode.CapturesFiles() {
		return nil
	}
	window, err := strconv.ParseUint(state.UpdatesFrom, 10, 64)
	if err != nil || strconv.FormatUint(window, 10) != state.UpdatesFrom {
		return fmt.Errorf("explanation reminder has an invalid reporting window")
	}
	allowed := make(map[string]bool, len(paths))
	for _, path := range paths {
		allowed[path] = true
	}
	seen := make(map[string]bool)
	pending := make([]telemetry.PendingChange, 0, len(paths))
	for _, change := range state.PendingChanges {
		id, err := strconv.ParseUint(change.ChangeID, 10, 64)
		if err != nil || id == 0 || id < window || strconv.FormatUint(id, 10) != change.ChangeID || !allowed[change.NodeName] || seen[change.ChangeID] {
			continue
		}
		if change.Action != "created" && change.Action != "modified" && change.Action != "deleted" {
			continue
		}
		seen[change.ChangeID] = true
		pending = append(pending, change)
		if len(pending) == 50 {
			break
		}
	}
	if len(pending) == 0 && omittedPaths == 0 {
		return nil
	}
	context := reminderContext(delivered.root, endpoint, delivered.activity, state.UpdateMode, state.UpdatesFrom, pending, state.PendingCount, omittedPaths)
	if context == "" {
		return nil
	}
	return writeHookContext(output, "PostToolUse", context)
}

func writeHookContext(output io.Writer, event, context string) error {
	return json.NewEncoder(output).Encode(struct {
		Output struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}{Output: struct {
		Event   string `json:"hookEventName"`
		Context string `json:"additionalContext"`
	}{event, context}})
}

// reminderPaths accepts concrete file-edit schemas only. Terminal commands,
// readers, MCP calls, and other clients never trigger a model reminder.
func reminderPaths(clientName string, raw []byte, root string) ([]string, int) {
	if clientName != "codex" && clientName != "claude" {
		return nil, 0
	}
	var input struct {
		Event string          `json:"hook_event_name"`
		CWD   string          `json:"cwd"`
		Tool  string          `json:"tool_name"`
		Input json.RawMessage `json:"tool_input"`
	}
	if json.Unmarshal(raw, &input) != nil || input.Event != "PostToolUse" || input.CWD == "" {
		return nil, 0
	}
	cwd, err := filepath.EvalSymlinks(input.CWD)
	if err != nil || !cursorWithin(root, cwd) {
		return nil, 0
	}
	var args struct {
		Command  string `json:"command"`
		Patch    string `json:"patch"`
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
	}
	var candidates []string
	switch clientName {
	case "codex":
		if input.Tool != "apply_patch" {
			return nil, 0
		}
		if json.Unmarshal(input.Input, &args) != nil && json.Unmarshal(input.Input, &args.Command) != nil {
			return nil, 0
		}
		patch := args.Command
		if patch == "" {
			patch = args.Patch
		}
		for _, match := range reminderPatchPath.FindAllStringSubmatch(patch, -1) {
			candidates = append(candidates, match[1])
		}
		if len(candidates) == 0 {
			candidates = append(candidates, args.FilePath, args.Path)
		}
	case "claude":
		if input.Tool != "Edit" && input.Tool != "Write" && input.Tool != "MultiEdit" {
			return nil, 0
		}
		if json.Unmarshal(input.Input, &args) != nil {
			return nil, 0
		}
		candidates = append(candidates, args.FilePath)
	}
	confined, err := os.OpenRoot(root)
	if err != nil {
		return nil, 0
	}
	defer confined.Close()
	seen := make(map[string]bool)
	paths := make([]string, 0, len(candidates))
	omitted := 0
	for _, file := range candidates {
		if file == "" || strings.ContainsAny(file, "#\\") || strings.IndexFunc(file, unicode.IsControl) >= 0 {
			continue
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(cwd, file)
		}
		relative, err := filepath.Rel(root, file)
		if err != nil {
			continue
		}
		relative = filepath.ToSlash(relative)
		node, err := telemetry.Node(relative)
		if err != nil || node != relative || seen[node] {
			continue
		}
		if _, err := confined.Stat(filepath.FromSlash(relative)); err != nil && !os.IsNotExist(err) {
			continue
		}
		seen[node] = true
		if len(paths) < 50 {
			paths = append(paths, node)
		} else {
			omitted++
		}
	}
	return paths, omitted
}

func reminderContext(root, endpoint string, activity telemetry.Activity, mode telemetry.UpdateMode, window string, pending []telemetry.PendingChange, total, omittedPaths int) string {
	label := "Every file"
	if mode == telemetry.UpdatesActivity {
		label = "All activity"
	}
	command := ""
	if executable, err := os.Executable(); err == nil {
		command = " If stef MCP tools are unavailable, send the same report_changes JSON on stdin to: " + reminderQuote(executable) + " --report-changes --repo " + reminderQuote(root) + " --daemon " + reminderQuote(endpoint) + "."
	}
	if total < len(pending) {
		total = len(pending)
	}
	for {
		incomplete := total > len(pending) || omittedPaths > 0
		guidance := " Use these exact IDs without another get_state."
		if incomplete {
			guidance = " This reminder is incomplete. Report included IDs first. Recover remaining own IDs by POST to " + strings.TrimSuffix(endpoint, "/") + "/api/explanation-reminder with JSON {session_id: the metadata session_id, paths: remaining relative paths from this actual edit, settle_ms: 0}, at most 50 paths per request. This filters unrelated pending files before the limit. Report returned exact IDs; query again only after an accepted batch makes progress or for the next path batch. Do not poll or backfill earlier windows."
		}
		request := "stef: These saved revisions need your explanation (" + label + "). Call report_changes immediately, before another edit, test, or final reply."
		if len(pending) == 0 {
			request = "stef: This bounded reminder omits edit paths or revisions (" + label + "). Use the path-filtered recovery below before another edit, test, or final reply."
		}
		prefix := request + " Reuse a brief public explanation as common summary/why for matching change_id entries; override different file reasons. Optional parents connect related paths. Explain only your own changes; do not repeat the prose in chat." + guidance + " Honor subsequent mode receipts. If the host shows a truncated context preview, read the complete reminder before assuming this list is complete." + command + " Metadata: "
		metadata, _ := json.Marshal(struct {
			Repo             string                    `json:"repo"`
			SessionID        string                    `json:"session_id"`
			AgentID          string                    `json:"agent_id"`
			UpdatesFrom      string                    `json:"updates_from"`
			Changes          []telemetry.PendingChange `json:"pending_changes"`
			PendingCount     int                       `json:"pending_count"`
			OmittedRevisions int                       `json:"omitted_revisions"`
			OmittedPaths     int                       `json:"edited_paths_omitted"`
			Incomplete       bool                      `json:"incomplete"`
		}{root, activity.SessionID, activity.AgentID, window, pending, total, total - len(pending), omittedPaths, incomplete})
		if len(prefix)+len(metadata) <= maxReminderBytes {
			return prefix + string(metadata)
		}
		if len(pending) == 0 {
			return ""
		}
		pending = pending[:len(pending)-1]
	}
}

func reminderQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
