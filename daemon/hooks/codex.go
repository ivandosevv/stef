package hooks

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

var ErrIgnored = errors.New("hook is irrelevant to workflow observation")
var patchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$`)
var readCommand = regexp.MustCompile(`(?:^|[;&|]\s*)\s*(?:cat|rg|grep|sed|head|tail|ls|find|pwd|wc)\b`)
var failedExit = regexp.MustCompile(`(?m)^(?:Process exited with code|Exit code:)\s*([0-9]+)\b`)

// Codex adapts the documented local hook contract, not private transcripts.
func Codex(raw []byte, root string) (telemetry.Activity, error) {
	var input struct {
		Event     string          `json:"hook_event_name"`
		SessionID string          `json:"session_id"`
		CWD       string          `json:"cwd"`
		Tool      string          `json:"tool_name"`
		Input     json.RawMessage `json:"tool_input"`
		Response  json.RawMessage `json:"tool_response"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return telemetry.Activity{}, err
	}
	if input.CWD == "" {
		return telemetry.Activity{}, fmt.Errorf("Codex hook requires cwd")
	}
	cwd, err := filepath.EvalSymlinks(input.CWD)
	if err != nil {
		return telemetry.Activity{}, fmt.Errorf("resolve hook cwd: %w", err)
	}
	rel, err := filepath.Rel(root, cwd)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return telemetry.Activity{}, fmt.Errorf("hook cwd is outside the tracked root")
	}
	if strings.HasPrefix(input.Tool, "mcp__stef__") {
		return telemetry.Activity{}, ErrIgnored
	}
	sum := sha256.Sum256([]byte(input.SessionID))
	id := "codex"
	if input.SessionID != "" {
		id = fmt.Sprintf("codex-%x", sum[:4])
	}
	a := telemetry.Activity{AgentID: id, Tool: input.Tool}
	if input.SessionID != "" {
		a.SessionID = fmt.Sprintf("codex-%x", sum[:16])
	}
	var args struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
	}
	if len(input.Input) > 0 && string(input.Input) != "null" {
		if err := json.Unmarshal(input.Input, &args); err != nil {
			// Some freeform tools supply their command as a JSON string.
			if err := json.Unmarshal(input.Input, &args.Command); err != nil {
				return a, fmt.Errorf("unsupported tool input")
			}
		}
	}
	switch input.Event {
	case "SessionStart", "UserPromptSubmit":
		a.Phase = "started"
	case "Stop", "SessionEnd":
		a.Phase = "completed"
	case "PreToolUse":
		tool := strings.ToLower(input.Tool)
		switch {
		case strings.Contains(tool, "patch") || tool == "edit" || tool == "write":
			a.Phase = "editing"
		case strings.Contains(tool, "read") || tool == "grep" || tool == "glob":
			a.Phase = "reading"
		default:
			a.Phase = "terminal"
		}
		if tool == "bash" || strings.HasSuffix(tool, "exec_command") {
			if readCommand.MatchString(args.Command) {
				a.Phase = "reading"
			}
			for _, test := range []string{"go test", "npm test", "npm run test", "pytest", "cargo test", "gradle test", "./gradlew test", "mvn test", "python -m unittest", "python3 -m unittest"} {
				if strings.Contains(strings.ToLower(args.Command), test) {
					a.Phase = "testing"
					break
				}
			}
		}
	case "PostToolUse":
		a.Phase = "tool_done"
		if codexFailure(input.Response) {
			a.Phase = "failed"
		}
	default:
		return a, ErrIgnored
	}
	file := args.FilePath
	if file == "" {
		file = args.Path
	}
	if file == "" && strings.Contains(strings.ToLower(input.Tool), "patch") {
		matches := patchFile.FindAllStringSubmatch(args.Command, -1)
		if len(matches) == 1 {
			file = matches[0][1]
		}
	}
	if file != "" {
		if !filepath.IsAbs(file) {
			file = filepath.Join(cwd, file)
		}
		if relative, err := filepath.Rel(root, file); err == nil {
			if node, err := telemetry.Node(filepath.ToSlash(relative)); err == nil {
				a.NodeName = node
			}
		}
	}
	_, err = a.Event()
	return a, err
}

func codexFailure(raw json.RawMessage) bool {
	var response struct {
		IsError  bool   `json:"isError"`
		ExitCode *int   `json:"exit_code"`
		Output   string `json:"output"`
		Metadata struct {
			ExitCode *int `json:"exit_code"`
		} `json:"metadata"`
	}
	var text string
	if json.Unmarshal(raw, &response) == nil {
		if response.IsError || (response.ExitCode != nil && *response.ExitCode != 0) || (response.Metadata.ExitCode != nil && *response.Metadata.ExitCode != 0) {
			return true
		}
		text = response.Output
	} else {
		_ = json.Unmarshal(raw, &text)
	}
	if match := failedExit.FindStringSubmatch(text); len(match) == 2 {
		code, _ := strconv.Atoi(match[1])
		return code != 0
	}
	return false
}
