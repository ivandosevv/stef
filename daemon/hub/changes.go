package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func isFileChange(e telemetry.Event) bool {
	return e.Source == "watcher" && e.Kind == "file" && e.Category == "" && !strings.Contains(e.NodeName, "#") && (e.Action == "created" || e.Action == "modified" || e.Action == "deleted")
}

func (h *Hub) changeLocked(session, id string) (telemetry.Event, error) {
	for _, event := range h.sessions[session] {
		if event.ID == id && isFileChange(event) {
			return event, nil
		}
	}
	return telemetry.Event{}, fmt.Errorf("change_id %q is not a retained file change in this chat", id)
}

func (h *Hub) validateChangeLocked(e telemetry.Event) error {
	if e.ChangeID == "" {
		return nil
	}
	if (e.Source != "" && e.Source != "agent") || e.Category != "" {
		return fmt.Errorf("only agent explanations can link change_id")
	}
	target, err := h.changeLocked(e.SessionID, e.ChangeID)
	if err != nil {
		return err
	}
	if target.NodeName != e.NodeName || target.Action != e.Action {
		return fmt.Errorf("change_id must match the observed path and action")
	}
	if !h.currentEventLocked(target.ID, e.SessionID) {
		return fmt.Errorf("change_id %q predates the current reporting window; use context_id only for an explicit developer request", e.ChangeID)
	}
	return nil
}

func (h *Hub) currentEventLocked(id string, session ...string) bool {
	n, err := strconv.ParseUint(id, 10, 64)
	from := h.updatesFrom
	if len(session) > 0 {
		from = h.settingLocked(session[0]).From
	}
	return err == nil && n >= from
}

// PendingChanges returns unexplained revisions from the current reporting window, without code.
func (h *Hub) PendingChanges(session string, limit int) ([]telemetry.PendingChange, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session = h.resolveLocked(session)
	if !h.settingLocked(session).Mode.CapturesFiles() {
		return []telemetry.PendingChange{}, 0
	}
	if session == "" {
		session = h.active
	}
	if limit < 1 || limit > 50 {
		limit = 50
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
	changes := make([]telemetry.PendingChange, 0)
	count := 0
	for _, event := range h.sessions[session] {
		if !isFileChange(event) || explained[event.ID] || !h.currentEventLocked(event.ID, session) {
			continue
		}
		count++
		if len(changes) < limit {
			changes = append(changes, telemetry.PendingChange{ChangeID: event.ID, NodeName: event.NodeName, Action: event.Action})
		}
	}
	return changes, count
}

// ExplainChanges validates and persists a complete batch before publishing any of it.
// Holding the session lock also prevents Clear or another edit from changing its targets.
func (h *Hub) ExplainChanges(session, agent string, changes []telemetry.ChangeExplanation) ([]telemetry.Event, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session = h.resolveLocked(session)
	if !h.settingLocked(session).Mode.CapturesFiles() {
		return []telemetry.Event{}, nil
	}
	if len(changes) < 1 || len(changes) > 50 {
		return nil, fmt.Errorf("changes must contain 1–50 explanations")
	}
	if session != "" {
		if err := telemetry.Session(session); err != nil {
			return nil, err
		}
	}
	if session == "" {
		session = h.active
	}
	events := make([]telemetry.Event, 0, len(changes))
	seen := make(map[string]bool)
	var journal bytes.Buffer
	now := time.Now().UTC()
	for i, change := range changes {
		if change.ChangeID == "" || seen[change.ChangeID] {
			return nil, fmt.Errorf("each explanation requires a distinct change_id")
		}
		seen[change.ChangeID] = true
		target, err := h.changeLocked(session, change.ChangeID)
		if err != nil {
			return nil, err
		}
		if !h.currentEventLocked(target.ID, session) {
			return nil, fmt.Errorf("change_id %q predates the current reporting window; omit it from automatic batches, or use report_event context_id only for a developer-requested clarification", change.ChangeID)
		}
		event, err := telemetry.Normalize(telemetry.Event{SessionID: session, AgentID: agent, ChangeID: change.ChangeID, NodeName: target.NodeName, Action: target.Action, Summary: change.Summary, Why: change.Why, Parents: change.Parents, Status: "ready", Source: "agent"})
		if err != nil {
			return nil, err
		}
		h.recordWindowLocked(&event)
		event.ID = strconv.FormatUint(h.next+uint64(i)+1, 10)
		event.Timestamp = now.Format(time.RFC3339Nano)
		data, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			journal.WriteByte('\n')
		}
		journal.Write(data)
		events = append(events, event)
	}
	if err := h.persist(journal.Bytes()); err != nil {
		return nil, err
	}
	for _, event := range events {
		h.next++
		h.store(event)
		delete(h.claims[session+"\n"+event.NodeName], event.AgentID)
		h.broadcast(h.eventMessageLocked(event))
	}
	return events, nil
}
