package hub

import "github.com/ivandosevv/stef/daemon/telemetry"

// OfferFeedback includes at most three unseen human notes at a real agent check-in.
// Each note is offered once per agent during this engine lifetime; history remains visible.
func (h *Hub) OfferFeedback(agent string, session ...string) []telemetry.FeedbackNote {
	if agent == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.updateMode == telemetry.UpdatesDisabled {
		return nil
	}
	var notes []telemetry.FeedbackNote
	id := h.active
	if len(session) > 0 && session[0] != "" {
		id = session[0]
	}
	key := id + "\n" + agent
	seen := h.offered[key]
	for _, event := range h.sessions[id] {
		if event.Category != "feedback" || event.Source != "human" || (event.TargetAgent != "*" && event.TargetAgent != agent) || seen[event.ID] {
			continue
		}
		if seen == nil {
			seen = make(map[string]bool)
			h.offered[key] = seen
		}
		notes = append(notes, telemetry.FeedbackNote{ID: event.ID, NodeName: event.NodeName, Kind: event.NoteKind, Text: event.Why})
		seen[event.ID] = true
		if len(notes) == 3 {
			break
		}
	}
	return notes
}
