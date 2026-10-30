package telemetry

import (
	"fmt"
	"strings"
)

type Feedback struct {
	SessionID   string `json:"session_id,omitempty"`
	NodeName    string `json:"node_name"`
	TargetAgent string `json:"target_agent,omitempty"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
}
type FeedbackNote struct {
	ID       string `json:"id"`
	NodeName string `json:"node_name"`
	Kind     string `json:"kind"`
	Text     string `json:"text"`
}

func (f Feedback) Event() (Event, error) {
	summaries := map[string]string{"question": "Developer asked a question", "suggestion": "Developer suggested a change", "concern": "Developer flagged a concern"}
	summary, ok := summaries[f.Kind]
	if !ok {
		return Event{}, fmt.Errorf("kind must be question, suggestion, or concern")
	}
	target := strings.TrimSpace(f.TargetAgent)
	if target == "" {
		target = "*"
	}
	node := strings.TrimSpace(f.NodeName)
	if node == "" {
		node = "@workflow"
	}
	return Normalize(Event{SessionID: f.SessionID, AgentID: "developer", NodeName: node, Action: "scouted", Summary: summary, Why: f.Text, Status: "ready", Source: "human", Category: "feedback", NoteKind: f.Kind, TargetAgent: target, Kind: "feedback"})
}
