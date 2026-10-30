package hub

import (
	"fmt"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

// Context replies only answer an explicit developer request about a retained visible step.
// A requested file answer also satisfies that exact revision's pending rationale.
// Context replies do not capture code or alter node claims.
func (h *Hub) validateContextLocked(e telemetry.Event) error {
	if e.ContextID == "" {
		return nil
	}
	if (e.Source != "" && e.Source != "agent") || e.Category != "" || e.ChangeID != "" {
		return fmt.Errorf("only agent context explanations can link context_id")
	}
	for _, target := range h.sessions[e.SessionID] {
		if target.ID != e.ContextID {
			continue
		}
		if target.ChangeID != "" || target.ContextID != "" {
			break
		}
		milestone := (target.Source == "agent" || target.Source == "") && target.Category == ""
		hook := target.Source == "hook" && target.Category == "activity" && (target.Phase == "reading" || target.Phase == "editing" || target.Phase == "testing" || target.Phase == "failed")
		if target.NodeName == "@workflow" && target.Phase != "testing" && target.Phase != "failed" {
			hook = false
		}
		if !isFileChange(target) && !milestone && !hook {
			break
		}
		if target.NodeName != e.NodeName || target.Action != e.Action {
			return fmt.Errorf("context_id must match the retained step's path and action")
		}
		return nil
	}
	return fmt.Errorf("context_id %q is not a retained visible step in this chat", e.ContextID)
}
