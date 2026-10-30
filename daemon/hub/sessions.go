package hub

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

type sessionControl struct {
	Control    string               `json:"stef_control"`
	SessionID  string               `json:"session_id,omitempty"`
	UpdateMode telemetry.UpdateMode `json:"update_mode,omitempty"`
}

func (h *Hub) store(e telemetry.Event) {
	events := append(h.sessions[e.SessionID], e)
	if len(events) > HistoryLimit {
		events = append([]telemetry.Event(nil), events[len(events)-HistoryLimit:]...)
	}
	h.sessions[e.SessionID] = events
	if e.SessionID == h.active {
		h.events = events
	}
}

func (h *Hub) activate(id string) {
	if _, exists := h.sessions[id]; !exists {
		h.sessions[id] = []telemetry.Event{}
	}
	h.active = id
	h.events = h.sessions[id]
}

func (h *Hub) persist(data []byte) error {
	if h.journal == nil {
		return nil
	}
	start, err := h.journal.Seek(0, 2)
	if err != nil {
		return err
	}
	if _, err = h.journal.Write(append(data, '\n')); err == nil {
		err = h.journal.Sync()
	}
	if err != nil {
		_ = h.journal.Truncate(start)
		return fmt.Errorf("persist workflow: %w", err)
	}
	return nil
}

func (h *Hub) broadcast(message Message) {
	for client := range h.clients {
		select {
		case client <- message:
		default:
			close(client)
			delete(h.clients, client)
		}
	}
}

func (h *Hub) activateLocked(id string) error {
	if id == h.active {
		return nil
	}
	data, _ := json.Marshal(sessionControl{Control: "activate", SessionID: id})
	if err := h.persist(data); err != nil {
		return err
	}
	h.activate(id)
	h.broadcast(h.snapshot())
	return nil
}

// ActivateSession restores a chat's workflow, or starts empty for a new chat ID.
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

// ClearSession starts a new segment for one chat and keeps other chats intact.
func (h *Hub) ClearSession(id string) (Message, error) {
	if err := telemetry.Session(id); err != nil {
		return Message{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.sessions[id]; !exists {
		return Message{}, fmt.Errorf("unknown session")
	}
	data, _ := json.Marshal(sessionControl{Control: "clear", SessionID: id})
	if err := h.persist(data); err != nil {
		return Message{}, err
	}
	h.sessions[id] = []telemetry.Event{}
	for key := range h.claims {
		if strings.HasPrefix(key, id+"\n") {
			delete(h.claims, key)
		}
	}
	if id == h.active {
		h.events = h.sessions[id]
		h.broadcast(h.snapshot())
	}
	return Message{Type: "snapshot", Repo: h.repo, SessionID: id, UpdateMode: h.updateMode, Events: []telemetry.Event{}}, nil
}

func (h *Hub) SessionSnapshot(id string) Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	if id == "" {
		id = h.active
	}
	return Message{Type: "snapshot", Repo: h.repo, SessionID: id, UpdateMode: h.updateMode, Events: append([]telemetry.Event{}, h.sessions[id]...)}
}
