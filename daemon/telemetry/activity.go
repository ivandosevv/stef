package telemetry

import "fmt"

// Activity is a client/IDE observation. It contains no prompt, stdout, or hidden reasoning.
type Activity struct {
	SessionID string `json:"session_id,omitempty"`
	AgentID   string `json:"agent_id"`
	Phase     string `json:"phase"`
	NodeName  string `json:"node_name,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

var PhaseSummaries = map[string]string{
	"started": "Session started", "reading": "Reading project context", "searching": "Searching project context", "editing": "Applying code changes",
	"testing": "Running test command", "terminal": "Running terminal command", "tool_done": "Tool finished",
	"completed": "Agent turn finished", "failed": "Tool reported a failure",
}

func ValidPhase(phase string) bool { _, ok := PhaseSummaries[phase]; return ok }

func (a Activity) Event() (Event, error) {
	summary, ok := PhaseSummaries[a.Phase]
	if !ok {
		return Event{}, fmt.Errorf("unsupported activity phase %q", a.Phase)
	}
	if a.Summary != "" {
		summary = a.Summary
	}
	status := "running"
	if a.Phase == "tool_done" || a.Phase == "completed" || a.Phase == "failed" {
		status = "ready"
	}
	return Normalize(Event{SessionID: a.SessionID, AgentID: a.AgentID, NodeName: a.NodeName, Action: "scouted", Summary: summary, Why: "Observed a client lifecycle event; no decision rationale was supplied.", Status: status, Source: "hook", Category: "activity", Phase: a.Phase, Tool: a.Tool, Kind: "activity"})
}
