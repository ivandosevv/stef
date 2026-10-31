package hub

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func (h *Hub) Settings(session ...string) telemetry.Settings {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(session) > 0 {
		return telemetry.Settings{UpdateMode: h.settingLocked(session[0]).Mode}
	}
	return telemetry.Settings{UpdateMode: h.captureModeLocked()}
}
func (h *Hub) SettingsForSession(session string) telemetry.Settings { return h.Settings(session) }
func (h *Hub) captureModeLocked() telemetry.UpdateMode {
	if !h.workspaceEnabled {
		return h.updateMode
	}
	mode := telemetry.UpdatesDisabled
	for _, setting := range h.workspaceSettings {
		if setting.Mode == telemetry.UpdatesFiles {
			return telemetry.UpdatesFiles
		}
		if setting.Mode == telemetry.UpdatesMilestones {
			mode = telemetry.UpdatesMilestones
		}
	}
	return mode
}

// CaptureState records only when a workspace needs file revisions.
func (h *Hub) CaptureState() (telemetry.UpdateMode, uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.captureModeLocked(), h.captureEpoch
}

// SetUpdateMode changes one workspace when given an origin/session. Legacy
// unscoped calls retain the earlier root preference for old journals and clients.
func (h *Hub) SetUpdateMode(mode telemetry.UpdateMode, session ...string) (telemetry.Settings, error) {
	if !mode.Valid() {
		return telemetry.Settings{}, fmt.Errorf("update_mode must be milestones, files, or disabled")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(session) > 0 && session[0] != "" {
		if telemetry.Session(session[0]) != nil {
			return telemetry.Settings{}, fmt.Errorf("invalid session_id")
		}
		id := h.resolveLocked(session[0])
		if err := h.admitWorkspaceLocked(id); err != nil {
			return telemetry.Settings{}, err
		}
		setting := h.settingLocked(id)
		if mode != setting.Mode {
			captureBefore := h.captureModeLocked()
			data, _ := json.Marshal(sessionControl{Control: "settings", SessionID: id, UpdateMode: mode})
			if err := h.persist(data); err != nil {
				return telemetry.Settings{}, err
			}
			setting.Mode = mode
			setting.From = h.next + 1
			setting.CaptureFrom = h.next + 1
			h.workspaceSettings[id] = setting
			if (captureBefore == telemetry.UpdatesFiles) != (h.captureModeLocked() == telemetry.UpdatesFiles) {
				h.captureEpoch++
			}
			h.clearClaimsLocked(id)
			h.broadcast(h.snapshotSessionLocked(id))
		}
		return telemetry.Settings{UpdateMode: mode}, nil
	}
	if mode != h.updateMode {
		data, _ := json.Marshal(sessionControl{Control: "settings", UpdateMode: mode})
		if err := h.persist(data); err != nil {
			return telemetry.Settings{}, err
		}
		h.updateMode = mode
		h.updatesFrom = h.next + 1
		h.captureEpoch++
		h.claims = make(map[string]map[string]time.Time)
		h.broadcast(h.snapshot())
	}
	return telemetry.Settings{UpdateMode: mode}, nil
}
func (h *Hub) SetSessionUpdateMode(session string, mode telemetry.UpdateMode) (telemetry.Settings, error) {
	return h.SetUpdateMode(mode, session)
}
func (h *Hub) suppressLocked(e telemetry.Event) bool {
	mode := h.settingLocked(e.SessionID).Mode
	if e.Source == "watcher" && e.SessionID == "local" && h.workspaceEnabled {
		mode = h.captureModeLocked()
	}
	if mode == telemetry.UpdatesDisabled {
		return true
	}
	return mode == telemetry.UpdatesMilestones && (e.Source == "watcher" || e.Source == "hook" || e.ChangeID != "") && e.ContextID == ""
}
