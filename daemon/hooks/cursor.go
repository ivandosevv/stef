package hooks

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

// Cursor observes documented local post-tool and conversation lifecycle callbacks.
// It never reads transcripts or retains commands, outputs, prompts, or thoughts.
func Cursor(raw []byte, root string) (telemetry.Activity, error) {
	var input struct {
		Event          string   `json:"hook_event_name"`
		ConversationID string   `json:"conversation_id"`
		SessionID      string   `json:"session_id"`
		Roots          []string `json:"workspace_roots"`
		CWD            string   `json:"cwd"`
		Tool           string   `json:"tool_name"`
		Status         string   `json:"status"`
		Reason         string   `json:"reason"`
		Input          struct {
			FilePath         string `json:"file_path"`
			Path             string `json:"path"`
			WorkingDirectory string `json:"working_directory"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return telemetry.Activity{}, err
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return telemetry.Activity{}, fmt.Errorf("resolve tracked root: %w", err)
	}
	// A multiroot callback has no universal active-root identity. Avoid attributing
	// another folder's work to this project; explicit MCP reports still work there.
	if len(input.Roots) != 1 {
		return telemetry.Activity{}, fmt.Errorf("Cursor hook requires one unambiguous workspace root")
	}
	workspace, err := filepath.EvalSymlinks(input.Roots[0])
	if err != nil || workspace != canonical {
		return telemetry.Activity{}, fmt.Errorf("Cursor hook workspace does not match the tracked root")
	}
	for _, directory := range []string{input.CWD, input.Input.WorkingDirectory} {
		if directory == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(directory)
		if err != nil || !cursorWithin(canonical, resolved) {
			return telemetry.Activity{}, fmt.Errorf("Cursor hook cwd is outside the tracked root")
		}
	}
	if cursorOwnTool(input.Tool) {
		return telemetry.Activity{}, ErrIgnored
	}
	conversation := input.ConversationID
	if conversation == "" {
		conversation = input.SessionID
	}
	if conversation == "" {
		return telemetry.Activity{}, fmt.Errorf("Cursor hook requires conversation_id")
	}
	sum := sha256.Sum256([]byte(conversation))
	activity := telemetry.Activity{AgentID: fmt.Sprintf("cursor-%x", sum[:4]), SessionID: fmt.Sprintf("cursor-%x", sum[:16]), Tool: hookToolLabel(input.Tool)}
	switch input.Event {
	case "sessionStart":
		activity.Phase = "started"
	case "sessionEnd", "stop":
		activity.Phase = "completed"
		if input.Status == "error" || input.Reason == "error" {
			activity.Phase = "failed"
		}
	case "postToolUse":
		activity.Phase = "tool_done"
	case "postToolUseFailure":
		activity.Phase = "failed"
	default:
		return telemetry.Activity{}, ErrIgnored
	}
	file := input.Input.FilePath
	if file == "" {
		file = input.Input.Path
	}
	if file != "" {
		cwd := canonical
		if input.CWD != "" {
			cwd = input.CWD
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(cwd, file)
		}
		if !cursorWithin(canonical, file) {
			return telemetry.Activity{}, ErrIgnored
		}
		relative, err := filepath.Rel(canonical, file)
		if err != nil {
			return telemetry.Activity{}, ErrIgnored
		}
		confined, err := os.OpenRoot(canonical)
		if err != nil {
			return telemetry.Activity{}, err
		}
		_, err = confined.Stat(relative)
		confined.Close()
		if err != nil && !os.IsNotExist(err) {
			return telemetry.Activity{}, ErrIgnored
		}
		node, err := telemetry.Node(filepath.ToSlash(relative))
		if err != nil {
			return telemetry.Activity{}, ErrIgnored
		}
		activity.NodeName = node
	}
	_, err = activity.Event()
	return activity, err
}

func cursorWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func cursorOwnTool(tool string) bool {
	lower := strings.ToLower(tool)
	for _, prefix := range []string{"mcp__stef__", "mcp:stef:", "mcp:stef/", "mcp_stef_"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	if strings.HasPrefix(lower, "mcp:") {
		for _, name := range []string{"get_state", "report_event", "report_changes", "set_session", "set_workspace"} {
			if strings.TrimPrefix(lower, "mcp:") == name {
				return true
			}
		}
	}
	return false
}
