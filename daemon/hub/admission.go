package hub

import "fmt"

// Includes the internal local workflow, private chats, shared rooms and saved
// per-workspace metadata. Never discard an existing private history to make room.
const MaxWorkspaces = 1024

func (h *Hub) admitWorkspaceLocked(ids ...string) error {
	if len(ids) > 0 {
		allKnown := true
		for _, id := range ids {
			if id == "" {
				continue
			}
			_, session := h.sessions[id]
			_, named := h.workspaceNames[id]
			_, setting := h.workspaceSettings[id]
			_, origin := h.originFrom[id]
			if !session && !named && !setting && !origin && !h.shared[id] {
				allKnown = false
				break
			}
		}
		if allKnown {
			return nil // Ordinary check-ins/reports do not rebuild the catalog.
		}
	}
	known := make(map[string]bool, len(h.sessions))
	for id := range h.sessions {
		known[id] = true
	}
	for id := range h.workspaceNames {
		known[id] = true
	}
	for id := range h.workspaceSettings {
		known[id] = true
	}
	for id := range h.originFrom {
		known[id] = true
	}
	for id := range h.shared {
		known[id] = true
	}
	for _, id := range ids {
		if id != "" {
			known[id] = true
		}
	}
	if len(known) > MaxWorkspaces {
		return fmt.Errorf("history reached the %d-workspace limit; existing histories are preserved, use a fresh data directory for new chats", MaxWorkspaces)
	}
	return nil
}
