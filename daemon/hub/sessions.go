package hub

import (
	"encoding/json"
	"fmt"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

type sessionControl struct {
	Control       string               `json:"stef_control"`
	SessionID     string               `json:"session_id,omitempty"`
	UpdateMode    telemetry.UpdateMode `json:"update_mode,omitempty"`
	WorkspaceID   string               `json:"workspace_id,omitempty"`
	WorkspaceName string               `json:"workspace_name,omitempty"`
	Action        string               `json:"action,omitempty"`
	ObservationID string               `json:"observation_id,omitempty"`
	CapturedEvent *telemetry.Event     `json:"captured_event,omitempty"`
}

func (h *Hub) store(e telemetry.Event) {
	events := append(h.sessions[e.SessionID], e)
	if len(events) > HistoryLimit {
		events = append([]telemetry.Event(nil), events[len(events)-HistoryLimit:]...)
	}
	h.sessions[e.SessionID] = events
	if e.SessionID == h.resolveLocked(h.active) {
		h.events = events
	}
}
func (h *Hub) activate(id string) {
	if _, exists := h.sessions[id]; !exists {
		h.sessions[id] = []telemetry.Event{}
	}
	h.active = id
	h.events = h.sessions[h.resolveLocked(id)]
}
func (h *Hub) persist(data []byte) error {
	if h.journal == nil {
		return nil
	}
	start, err := h.journal.Seek(0, 2)
	if err != nil {
		return err
	}
	if _, err = h.journal.Write(append(data, 10)); err == nil {
		err = h.journal.Sync()
	}
	if err != nil {
		_ = h.journal.Truncate(start)
		return fmt.Errorf("persist workflow: %w", err)
	}
	return nil
}
func (h *Hub) broadcast(message Message) {
	if message.Type != "workspaces" && message.SessionID == h.resolveLocked(h.active) {
		for client := range h.clients {
			select {
			case client <- message:
			default:
				close(client)
				delete(h.clients, client)
			}
		}
	}
	for client, origin := range h.scopedClients {
		if message.Type != "workspaces" && message.SessionID != h.resolveLocked(origin) {
			continue
		}
		select {
		case client <- message:
		default:
			close(client)
			delete(h.scopedClients, client)
		}
	}
}
func (h *Hub) activateLocked(id string) error {
	if id == h.active {
		return nil
	}
	if err := h.ensureWorkspaceNameLocked(id); err != nil {
		return err
	}
	data, _ := json.Marshal(sessionControl{Control: "activate", SessionID: id})
	if err := h.persist(data); err != nil {
		return err
	}
	h.activate(id)
	h.broadcast(h.snapshot())
	return nil
}

// ActivateSession retains legacy selection behavior. Workspace-aware clients
// call RegisterSession once and use a pinned subscription thereafter.
func (h *Hub) ActivateSession(id string) (Message, error) {
	if err := telemetry.Session(id); err != nil {
		return Message{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.activateLocked(id); err != nil {
		return Message{}, err
	}
	return h.snapshot(), nil
}
func (h *Hub) ClearSession(id string) (Message, error) {
	if err := telemetry.Session(id); err != nil {
		return Message{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	id = h.resolveLocked(id)
	if _, exists := h.sessions[id]; !exists {
		return Message{}, fmt.Errorf("unknown session")
	}
	data, _ := json.Marshal(sessionControl{Control: "clear", SessionID: id})
	if err := h.persist(data); err != nil {
		return Message{}, err
	}
	h.sessions[id] = []telemetry.Event{}
	h.clearClaimsLocked(id)
	if setting, ok := h.workspaceSettings[id]; ok {
		setting.CaptureFrom = h.next + 1
		h.workspaceSettings[id] = setting
	}
	if id == h.resolveLocked(h.active) {
		h.events = h.sessions[id]
	}
	snapshot := h.snapshotSessionLocked(id)
	h.broadcast(snapshot)
	return snapshot, nil
}
func (h *Hub) SessionSnapshot(id string) Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.snapshotSessionLocked(id)
}
