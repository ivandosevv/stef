package hooks

import (
	"regexp"
	"strings"
)

var searchCommand = regexp.MustCompile(`(?:^|[;&|]\s*)\s*(?:rg|grep|find)\b`)
var readCommand = regexp.MustCompile(`(?:^|[;&|]\s*)\s*(?:cat|sed|head|tail|ls|pwd|wc)\b`)
var testCommand = regexp.MustCompile(`(?:^|[;&|]\s*)\s*(?:go\s+test|npm\s+(?:test|run\s+test(?::[^\s;&|]+)?)|pytest|cargo\s+test|gradle\s+test|\./gradlew\s+test|mvn\s+test|python3?\s+-m\s+(?:unittest|pytest))\b`)

// hookToolLabel preserves supported tool names without copying arbitrary
// client metadata into the visible workflow. MCP providers stay generic.
func hookToolLabel(tool string) string {
	lower := strings.ToLower(tool)
	if strings.HasPrefix(lower, "mcp:") || strings.HasPrefix(lower, "mcp__") {
		return "MCP"
	}
	for _, label := range []string{"Shell", "Bash", "Read", "Write", "Edit", "MultiEdit", "Grep", "Glob", "Search", "Delete", "Task", "apply_patch", "exec_command", "write_stdin"} {
		if strings.EqualFold(tool, label) {
			return label
		}
	}
	if tool == "" {
		return ""
	}
	return "Tool"
}

func searchTool(tool string) bool {
	switch strings.ToLower(tool) {
	case "grep", "glob", "search":
		return true
	default:
		return false
	}
}

// commandActivityPhase classifies a local command's observed intent only.
// Callers retain neither the command nor a claim about its result.
func commandActivityPhase(command string) string {
	switch {
	case testCommand.MatchString(command):
		return "testing"
	case searchCommand.MatchString(command):
		return "searching"
	case readCommand.MatchString(command):
		return "reading"
	default:
		return "terminal"
	}
}
