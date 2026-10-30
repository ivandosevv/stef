// Package hooks adapts explicit client lifecycle events without any model call.
package hooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

type ClaudeInput struct {
	Event     string `json:"hook_event_name"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	CWD       string `json:"cwd"`
	Tool      string `json:"tool_name"`
	Input     struct {
		FilePath string `json:"file_path"`
		Command  string `json:"command"`
	} `json:"tool_input"`
}

func Claude(raw []byte, root string) (telemetry.Activity, error) {
	var input ClaudeInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return telemetry.Activity{}, err
	}
	if cursorOwnTool(input.Tool) {
		return telemetry.Activity{}, ErrIgnored
	}
	if input.CWD != "" {
		cwd, err := filepath.EvalSymlinks(input.CWD)
		if err != nil || cwd != root {
			return telemetry.Activity{}, fmt.Errorf("hook cwd does not match the tracked root")
		}
	}
	// Claude documents agent_id on callbacks inside subagents and teammates.
	// A name/type alone is not a unique worker identity. Preserve parent IDs.
	identity := input.SessionID
	if input.AgentID != "" {
		identity += "\x00" + input.AgentID
	}
	sum := sha256.Sum256([]byte(identity))
	id := fmt.Sprintf("claude-%x", sum[:4])
	if identity == "" {
		id = "claude"
	}
	a := telemetry.Activity{AgentID: id, Tool: input.Tool}
	if identity != "" {
		a.SessionID = fmt.Sprintf("claude-%x", sum[:16])
	}
	switch input.Event {
	case "SessionStart":
		a.Phase = "started"
	case "Stop":
		a.Phase = "completed"
	case "PostToolUse":
		a.Phase = "tool_done"
	case "PostToolUseFailure":
		a.Phase = "failed"
	case "PreToolUse":
		tool := strings.ToLower(input.Tool)
		switch {
		case strings.Contains(tool, "read") || tool == "grep" || tool == "glob":
			a.Phase = "reading"
		case strings.Contains(tool, "edit") || strings.Contains(tool, "write") || strings.Contains(tool, "patch"):
			a.Phase = "editing"
		default:
			a.Phase = "terminal"
		}
		// A command classification reports intent to run tests, never an inferred result.
		command := strings.ToLower(input.Input.Command)
		for _, pattern := range []string{"go test", "npm test", "npm run test", "pytest", "cargo test", "gradle test", "./gradlew test", "mvn test"} {
			if strings.Contains(command, pattern) {
				a.Phase = "testing"
				break
			}
		}
	default:
		return a, fmt.Errorf("unsupported Claude hook event %q", input.Event)
	}
	if input.Input.FilePath != "" {
		file := input.Input.FilePath
		if !filepath.IsAbs(file) {
			file = filepath.Join(root, file)
		}
		relative, err := filepath.Rel(root, file)
		if err == nil {
			if n, err := telemetry.Node(filepath.ToSlash(relative)); err == nil {
				a.NodeName = n
			}
		}
	}
	_, err := a.Event()
	return a, err
}
func Send(ctx context.Context, endpoint, clientName string, input io.Reader) error {
	_, err := forward(ctx, endpoint, clientName, input, false)
	return err
}

type delivery struct {
	raw        []byte
	root       string
	activity   telemetry.Activity
	httpClient *http.Client
}

// forward is shared by observation-only callers and model-visible reminders.
// It sends one sanitized lifecycle observation, never raw hook input.
func forward(ctx context.Context, endpoint, clientName string, input io.Reader, strict bool) (delivery, error) {
	var delivered delivery
	if err := mcp.ValidateEndpoint(endpoint); err != nil {
		return delivered, err
	}
	raw, err := io.ReadAll(io.LimitReader(input, 256*1024+1))
	if err != nil {
		return delivered, err
	}
	if len(raw) > 256*1024 {
		return delivered, fmt.Errorf("hook payload exceeds 256 KiB")
	}
	httpClient := &http.Client{Timeout: 750 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var activity telemetry.Activity
	base := strings.TrimSuffix(endpoint, "/")
	if clientName == "claude" || clientName == "codex" || clientName == "cursor" {
		request, err := http.NewRequestWithContext(ctx, "GET", base+"/healthz", nil)
		if err != nil {
			return delivered, err
		}
		response, err := httpClient.Do(request)
		if err != nil {
			return delivered, err
		}
		var health struct {
			Repo    string `json:"repo"`
			Name    string `json:"name"`
			Status  string `json:"status"`
			Version string `json:"version"`
		}
		err = json.NewDecoder(response.Body).Decode(&health)
		response.Body.Close()
		if err != nil {
			return delivered, err
		}
		if response.StatusCode != http.StatusOK || health.Name != "stef" || health.Status != "ok" || (strict && health.Version != "0.3.0") {
			return delivered, fmt.Errorf("endpoint is not a compatible stef companion")
		}
		if !filepath.IsAbs(health.Repo) {
			return delivered, fmt.Errorf("companion root must be absolute")
		}
		root, err := filepath.EvalSymlinks(health.Repo)
		if err != nil {
			return delivered, fmt.Errorf("resolve companion root: %w", err)
		}
		delivered.root = root
		if clientName == "codex" {
			activity, err = Codex(raw, root)
		} else if clientName == "cursor" {
			activity, err = Cursor(raw, root)
		} else {
			activity, err = Claude(raw, root)
		}
		if errors.Is(err, ErrIgnored) {
			return delivered, nil
		}
		if err != nil {
			return delivered, err
		}
	} else if clientName == "generic" {
		if err := json.Unmarshal(raw, &activity); err != nil {
			return delivered, err
		}
	} else {
		return delivered, fmt.Errorf("unsupported hook client %q", clientName)
	}
	if _, err := activity.Event(); err != nil {
		return delivered, err
	}
	data, err := json.Marshal(activity)
	if err != nil {
		return delivered, err
	}
	request, err := http.NewRequestWithContext(ctx, "POST", base+"/api/activity", bytes.NewReader(data))
	if err != nil {
		return delivered, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		return delivered, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return delivered, fmt.Errorf("activity rejected (HTTP %d)", response.StatusCode)
	}
	delivered.raw, delivered.activity, delivered.httpClient = raw, activity, httpClient
	return delivered, nil
}
