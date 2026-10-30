package hub

import (
	"strconv"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

// ExplanationReminder reads one atomic reporting snapshot for the edited paths.
// It neither publishes events nor offers developer notes to an agent.
func (h *Hub) ExplanationReminder(session string, paths []string) (telemetry.ReminderState, error) {
	if session != "" {
		if err := telemetry.Session(session); err != nil {
			return telemetry.ReminderState{}, err
		}
	}
	files, err := telemetry.ReminderPaths(paths)
	if err != nil {
		return telemetry.ReminderState{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if session == "" {
		session = h.active
	}
	state := telemetry.ReminderState{
		Repo: h.repo, SessionID: session, UpdateMode: h.updateMode,
		UpdatesFrom:    strconv.FormatUint(h.updatesFrom, 10),
		PendingChanges: []telemetry.PendingChange{},
	}
	if h.updateMode != telemetry.UpdatesFiles {
		return state, nil
	}
	explained := make(map[string]bool)
	for _, event := range h.sessions[session] {
		if event.Source == "agent" {
			if event.ChangeID != "" {
				explained[event.ChangeID] = true
			}
			if event.ContextID != "" {
				explained[event.ContextID] = true
			}
		}
	}
	for _, event := range h.sessions[session] {
		if !files[event.NodeName] || !isFileChange(event) || explained[event.ID] || !h.currentEventLocked(event.ID) {
			continue
		}
		state.PendingCount++
		if len(state.PendingChanges) < telemetry.ReminderLimit {
			state.PendingChanges = append(state.PendingChanges, telemetry.PendingChange{ChangeID: event.ID, NodeName: event.NodeName, Action: event.Action})
		}
	}
	return state, nil
}
