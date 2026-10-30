package hub

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func (h *Hub) Settings() telemetry.Settings {
	h.mu.Lock()
	defer h.mu.Unlock()
	return telemetry.Settings{UpdateMode: h.updateMode}
}

// CaptureState lets the watcher discard queued saves across a mode boundary.
// The generation is internal and never participates in persisted chat history.
func (h *Hub) CaptureState() (telemetry.UpdateMode, uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.updateMode, h.captureEpoch
}

// SetUpdateMode persists a root-wide preference and refreshes every live view.
// History is retained; only future activity is subject to the new policy.
func (h *Hub) SetUpdateMode(mode telemetry.UpdateMode) (telemetry.Settings, error) {
	if !mode.Valid() {
		return telemetry.Settings{}, fmt.Errorf("update_mode must be milestones, files, or disabled")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if mode != h.updateMode {
		data, _ := json.Marshal(sessionControl{Control: "settings", UpdateMode: mode})
		if err := h.persist(data); err != nil {
			return telemetry.Settings{}, err
		}
		h.updateMode = mode
		h.captureEpoch++
		// Claims cannot survive a period when ownership changes are not recorded.
		h.claims = make(map[string]map[string]time.Time)
		h.broadcast(h.snapshot())
	}
	return telemetry.Settings{UpdateMode: h.updateMode}, nil
}

func (h *Hub) suppressLocked(e telemetry.Event) bool {
	if h.updateMode == telemetry.UpdatesDisabled {
		return true
	}
	return h.updateMode == telemetry.UpdatesMilestones && (e.Source == "watcher" || e.Source == "hook" || e.ChangeID != "")
}
