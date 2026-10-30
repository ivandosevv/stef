// Package telemetry defines the wire format shared by watchers, MCP, and the HUD.
package telemetry

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxText = 16384

type Event struct {
	SessionID   string   `json:"session_id,omitempty"`
	ID          string   `json:"id"`
	ChangeID    string   `json:"change_id,omitempty" jsonschema:"exact whole-file observation ID explained by this event"`
	ContextID   string   `json:"context_id,omitempty" jsonschema:"retained visible step ID; only for developer-requested additional context"`
	Timestamp   string   `json:"timestamp"`
	AgentID     string   `json:"agent_id" jsonschema:"stable identifier of the reporting agent"`
	NodeName    string   `json:"node_name" jsonschema:"repository-relative path or path#symbol"`
	Action      string   `json:"action" jsonschema:"scouted, created, modified, deleted, or tested"`
	Summary     string   `json:"summary" jsonschema:"brief badge, at most five words"`
	Why         string   `json:"why" jsonschema:"one sentence explaining the actual decision"`
	Status      string   `json:"status" jsonschema:"running or ready; ready releases the node claim"`
	Source      string   `json:"source"`
	Category    string   `json:"category,omitempty"`
	Phase       string   `json:"phase,omitempty"`
	Tool        string   `json:"tool,omitempty"`
	TargetAgent string   `json:"target_agent,omitempty"`
	NoteKind    string   `json:"note_kind,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Language    string   `json:"language,omitempty"`
	Line        int      `json:"line,omitempty"`
	Before      string   `json:"before,omitempty"`
	After       string   `json:"after,omitempty"`
	Diff        string   `json:"diff,omitempty"`
	Parents     []string `json:"parents,omitempty"`
	Collisions  []string `json:"collisions,omitempty"`
}

func Normalize(e Event) (Event, error) {
	for _, link := range []struct{ name, value string }{{"change_id", e.ChangeID}, {"context_id", e.ContextID}} {
		if link.value == "" {
			continue
		}
		id, err := strconv.ParseUint(link.value, 10, 64)
		if err != nil || id == 0 || link.value[0] == '0' || strings.IndexFunc(link.value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return e, fmt.Errorf("%s must be a positive event ID", link.name)
		}
	}
	if e.ChangeID != "" && e.ContextID != "" {
		return e, fmt.Errorf("change_id and context_id cannot be combined")
	}
	if e.SessionID != "" {
		if err := Session(e.SessionID); err != nil {
			return e, err
		}
	}
	if e.Category != "" && e.Category != "activity" && e.Category != "feedback" {
		return e, fmt.Errorf("invalid event category")
	}
	if e.Category == "feedback" && (len(e.TargetAgent) > 80 || utf8.RuneCountInString(e.Why) > 500 || (e.NoteKind != "question" && e.NoteKind != "suggestion" && e.NoteKind != "concern")) {
		return e, fmt.Errorf("feedback requires a valid kind, target up to 80 bytes, and text up to 500 characters")
	}
	if e.Category == "activity" {
		if !ValidPhase(e.Phase) {
			return e, fmt.Errorf("invalid activity phase")
		}
		if e.NodeName == "" {
			e.NodeName = "@workflow"
		}
	} else if e.Phase != "" {
		return e, fmt.Errorf("phase requires an activity category")
	}
	if len(e.Tool) > 100 {
		return e, fmt.Errorf("tool must be at most 100 bytes")
	}
	e.AgentID = strings.TrimSpace(e.AgentID)
	if e.AgentID == "" || len(e.AgentID) > 80 {
		return e, fmt.Errorf("agent_id must contain 1–80 characters")
	}
	node, err := Node(e.NodeName)
	if err != nil {
		return e, err
	}
	e.NodeName = node
	switch e.Action {
	case "scouted", "created", "modified", "deleted", "tested":
	default:
		return e, fmt.Errorf("unknown action %q", e.Action)
	}
	e.Summary = strings.Join(strings.Fields(e.Summary), " ")
	if len(strings.Fields(e.Summary)) == 0 || len(strings.Fields(e.Summary)) > 5 || len(e.Summary) > 160 {
		return e, fmt.Errorf("summary must contain one to five words (at most 160 bytes)")
	}
	e.Why = strings.TrimSpace(e.Why)
	if e.Why == "" || len(e.Why) > 2000 {
		return e, fmt.Errorf("why must contain 1–2000 bytes")
	}
	if e.Status == "" {
		e.Status = "running"
	}
	if e.Status != "running" && e.Status != "ready" {
		return e, fmt.Errorf("status must be running or ready")
	}
	if len(e.Before) > MaxText || len(e.After) > MaxText || len(e.Diff) > MaxText {
		return e, fmt.Errorf("code fields must each be at most %d bytes", MaxText)
	}
	if len(e.Kind) > 80 || len(e.Language) > 40 || e.Line < 0 {
		return e, fmt.Errorf("invalid kind, language, or line")
	}
	if len(e.Parents) > 20 {
		return e, fmt.Errorf("at most 20 parent nodes are allowed")
	}
	parents := make([]string, 0, len(e.Parents))
	for _, p := range e.Parents {
		n, err := Node(p)
		if err != nil {
			return e, err
		}
		parents = append(parents, n)
	}
	e.Parents = parents
	e.Collisions = nil
	return e, nil
}

func Session(id string) error {
	if !utf8.ValidString(id) || len(id) == 0 || len(id) > 160 || strings.IndexFunc(id, unicode.IsControl) >= 0 || strings.TrimSpace(id) != id {
		return fmt.Errorf("session_id must contain 1–160 bytes without control characters or surrounding spaces")
	}
	return nil
}

func Node(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	file, symbol, hasSymbol := strings.Cut(value, "#")
	if file == "" || len(value) > 500 || strings.HasPrefix(file, "/") || strings.Contains(file, ":") || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("node_name must be a repository-relative path, optionally followed by #symbol")
	}
	for _, part := range strings.Split(file, "/") {
		if part == ".." {
			return "", fmt.Errorf("node paths cannot traverse outside the repository")
		}
	}
	file = path.Clean(file)
	if file == "." {
		return "", fmt.Errorf("node_name must name a file")
	}
	if hasSymbol {
		if symbol == "" {
			return "", fmt.Errorf("symbol cannot be empty")
		}
		file += "#" + symbol
	}
	return file, nil
}
