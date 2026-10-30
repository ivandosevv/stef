package hub

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

// A workspace is a recorded workflow, not an MCP connection or hidden reasoning.
type Workspace struct {
	WorkspaceID string   `json:"workspace_id"`
	Shared      bool     `json:"shared"`
	Members     []string `json:"members"`
}
type WorkspaceInput struct {
	SessionID   string `json:"session_id"`
	Action      string `json:"action"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}
type WorkspaceRequest = WorkspaceInput
type WorkspaceReceipt struct {
	SessionID   string   `json:"session_id"`
	WorkspaceID string   `json:"workspace_id"`
	Shared      bool     `json:"shared"`
	Members     []string `json:"members"`
}
type workspaceSetting struct {
	Mode        telemetry.UpdateMode
	From        uint64
	CaptureFrom uint64
}

func (h *Hub) resolveLocked(id string) string {
	if id == "" {
		id = h.active
	}
	if room := h.memberships[id]; room != "" {
		return room
	}
	return id
}
func (h *Hub) ResolveSession(id string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.resolveLocked(id)
}
func (h *Hub) settingLocked(id string) workspaceSetting {
	id = h.resolveLocked(id)
	if setting, ok := h.workspaceSettings[id]; ok {
		return setting
	}
	return workspaceSetting{Mode: h.updateMode, From: h.updatesFrom}
}
func (h *Hub) snapshotSessionLocked(id string) Message {
	id = h.resolveLocked(id)
	setting := h.settingLocked(id)
	return Message{Type: "snapshot", Repo: h.repo, SessionID: id, WorkspaceID: id, UpdateMode: setting.Mode, UpdatesFrom: strconv.FormatUint(setting.From, 10), Events: append([]telemetry.Event{}, h.sessions[id]...), Workspaces: h.workspacesLocked()}
}
func (h *Hub) eventMessageLocked(event telemetry.Event) Message {
	setting := h.settingLocked(event.SessionID)
	return Message{Type: "event", SessionID: event.SessionID, WorkspaceID: event.SessionID, UpdateMode: setting.Mode, UpdatesFrom: strconv.FormatUint(setting.From, 10), Event: &event}
}

// RegisterSession gives an agent a durable private workflow. Reconnection alone
// does not call this with a new ID; clients retain their stable chat/worker ID.
func (h *Hub) RegisterSession(id string) (Message, error) {
	if err := telemetry.Session(id); err != nil {
		return Message{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.registerLocked(id); err != nil {
		return Message{}, err
	}
	if err := h.activateLocked(id); err != nil {
		return Message{}, err
	}
	return h.snapshotSessionLocked(id), nil
}
func (h *Hub) registerLocked(id string) error {
	if _, ok := h.originFrom[id]; ok {
		return nil
	}
	if h.shared[id] {
		return fmt.Errorf("session_id must identify an agent chat, not a shared workspace")
	}
	data, _ := json.Marshal(sessionControl{Control: "register", SessionID: id})
	if err := h.persist(data); err != nil {
		return err
	}
	h.applyRegisterLocked(id)
	h.broadcastWorkspacesLocked()
	return nil
}
func (h *Hub) applyRegisterLocked(id string) {
	captureBefore := h.captureModeLocked()
	h.workspaceEnabled = true
	h.originFrom[id] = h.next + 1
	if _, ok := h.sessions[id]; !ok {
		h.sessions[id] = []telemetry.Event{}
	}
	if _, ok := h.workspaceSettings[id]; !ok {
		setting := workspaceSetting{Mode: telemetry.UpdatesFiles, From: h.next + 1, CaptureFrom: h.next + 1}
		if len(h.sessions[id]) > 0 {
			setting.Mode = h.updateMode
			setting.From = h.updatesFrom
		}
		h.workspaceSettings[id] = setting
	}
	if (captureBefore == telemetry.UpdatesFiles) != (h.captureModeLocked() == telemetry.UpdatesFiles) {
		h.captureEpoch++
	}
}
func (h *Hub) workspaceLocked(id string) Workspace {
	item := Workspace{WorkspaceID: id, Shared: h.shared[id], Members: []string{}}
	if item.Shared {
		for origin, room := range h.memberships {
			if room == id {
				item.Members = append(item.Members, origin)
			}
		}
		sort.Strings(item.Members)
	} else {
		item.Members = []string{id}
	}
	return item
}
func (h *Hub) workspacesLocked() []Workspace {
	ids := make(map[string]bool)
	for id := range h.sessions {
		if id != "local" || !h.workspaceEnabled {
			ids[id] = true
		}
	}
	for id := range h.shared {
		ids[id] = true
	}
	result := make([]Workspace, 0, len(ids))
	for id := range ids {
		result = append(result, h.workspaceLocked(id))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].WorkspaceID < result[j].WorkspaceID })
	return result
}
func (h *Hub) Workspaces() []Workspace { h.mu.Lock(); defer h.mu.Unlock(); return h.workspacesLocked() }
func (h *Hub) broadcastWorkspacesLocked() {
	h.broadcast(Message{Type: "workspaces", Workspaces: h.workspacesLocked()})
}

// ConfigureWorkspace changes future routing only. A new shared room starts
// empty, and leaving restores the caller's untouched private history.
func (h *Hub) ConfigureWorkspace(input WorkspaceInput) (Workspace, error) {
	if err := telemetry.Session(input.SessionID); err != nil {
		return Workspace{}, err
	}
	if input.Action != "create" && input.Action != "join" && input.Action != "leave" {
		return Workspace{}, fmt.Errorf("action must be create, join, or leave")
	}
	if input.WorkspaceID != "" && telemetry.Session(input.WorkspaceID) != nil {
		return Workspace{}, fmt.Errorf("invalid workspace_id")
	}
	if input.Action != "join" && input.WorkspaceID != "" {
		return Workspace{}, fmt.Errorf("workspace_id is only accepted when joining")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if input.Action == "join" && !h.shared[input.WorkspaceID] {
		return Workspace{}, fmt.Errorf("join requires an existing shared workspace_id")
	}
	if err := h.registerLocked(input.SessionID); err != nil {
		return Workspace{}, err
	}
	room := input.WorkspaceID
	if input.Action == "create" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return Workspace{}, err
		}
		room = "shared-" + hex.EncodeToString(id[:])
	}
	if input.Action == "leave" {
		room = input.SessionID
	}
	if room == h.resolveLocked(input.SessionID) {
		return h.workspaceLocked(room), nil
	}
	data, _ := json.Marshal(sessionControl{Control: "workspace", SessionID: input.SessionID, WorkspaceID: room, Action: input.Action})
	if err := h.persist(data); err != nil {
		return Workspace{}, err
	}
	h.applyWorkspaceLocked(input.SessionID, room, input.Action)
	// The origin follows its own membership. Other origins keep their selection.
	h.broadcastWorkspacesLocked()
	h.broadcast(h.snapshotSessionLocked(room))
	return h.workspaceLocked(room), nil
}
func (h *Hub) applyWorkspaceLocked(origin, room, action string) {
	captureBefore := h.captureModeLocked()
	if action == "create" {
		h.shared[room] = true
		h.sessions[room] = []telemetry.Event{}
		h.workspaceSettings[room] = workspaceSetting{Mode: telemetry.UpdatesFiles, From: h.next + 1, CaptureFrom: h.next + 1}
	}
	if action == "leave" {
		delete(h.memberships, origin)
	} else {
		h.memberships[origin] = room
	}
	if (captureBefore == telemetry.UpdatesFiles) != (h.captureModeLocked() == telemetry.UpdatesFiles) {
		h.captureEpoch++
	}
	h.originFrom[origin] = h.next + 1
	if h.resolveLocked(h.active) == room {
		h.events = h.sessions[room]
	}
}

func (h *Hub) SubscribeSession(origin string) (<-chan Message, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if origin == "" {
		origin = h.active
	}
	client := make(chan Message, 128)
	client <- h.snapshotSessionLocked(origin)
	h.scopedClients[client] = origin
	return client, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.scopedClients[client]; ok {
			delete(h.scopedClients, client)
			close(client)
		}
	}
}

// ClaimChanges attaches only explicitly attributable paths to one workspace.
// Observation content remains exact; no author intent is inferred or generated.
// A neutral revision can belong to only one private or shared workspace.
func (h *Hub) ClaimChanges(origin string, paths []string) error {
	files, err := telemetry.ReminderPaths(paths)
	if err != nil {
		return err
	}
	if origin != "" && telemetry.Session(origin) != nil {
		return fmt.Errorf("invalid session_id")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.workspaceEnabled {
		return nil
	}
	if origin == "" {
		origin = h.active
	}
	room := h.resolveLocked(origin)
	setting := h.settingLocked(room)
	if setting.Mode != telemetry.UpdatesFiles {
		return nil
	}
	floor := h.originFrom[origin]
	if floor == 0 {
		return fmt.Errorf("set_session before capturing this agent's changes")
	}
	if setting.CaptureFrom > floor {
		floor = setting.CaptureFrom
	}
	if setting.From > floor {
		floor = setting.From
	}
	var journal bytes.Buffer
	clones := []telemetry.Event{}
	sourceIDs := []string{}
	for _, event := range h.sessions["local"] {
		n, parseErr := strconv.ParseUint(event.ID, 10, 64)
		if !isFileChange(event) || !files[event.NodeName] || parseErr != nil || n < floor || h.assigned[event.ID] != "" {
			continue
		}
		clone := event
		clone.SessionID = room
		clone.ID = strconv.FormatUint(h.next+uint64(len(clones))+1, 10)
		data, err := json.Marshal(sessionControl{Control: "capture", WorkspaceID: room, ObservationID: event.ID, CapturedEvent: &clone})
		if err != nil {
			return err
		}
		if journal.Len() > 0 {
			journal.WriteByte(10)
		}
		journal.Write(data)
		clones = append(clones, clone)
		sourceIDs = append(sourceIDs, event.ID)
	}
	if len(clones) == 0 {
		return nil
	}
	if err := h.persist(journal.Bytes()); err != nil {
		return err
	}
	for i, event := range clones {
		h.next++
		h.assigned[sourceIDs[i]] = room
		h.store(event)
		h.broadcast(h.eventMessageLocked(event))
	}
	return nil
}
func (h *Hub) CaptureChanges(origin string, paths []string) error {
	return h.ClaimChanges(origin, paths)
}
func (h *Hub) ClaimObservedChanges(origin string, paths []string) error {
	return h.ClaimChanges(origin, paths)
}

func (h *Hub) replayWorkspaceControl(control sessionControl) (bool, error) {
	switch control.Control {
	case "register":
		if telemetry.Session(control.SessionID) != nil {
			return true, fmt.Errorf("invalid workspace registration")
		}
		h.applyRegisterLocked(control.SessionID)
		return true, nil
	case "workspace":
		if telemetry.Session(control.SessionID) != nil || telemetry.Session(control.WorkspaceID) != nil || (control.Action != "create" && control.Action != "join" && control.Action != "leave") {
			return true, fmt.Errorf("invalid workspace membership")
		}
		h.applyWorkspaceLocked(control.SessionID, control.WorkspaceID, control.Action)
		return true, nil
	case "capture":
		if control.ObservationID == "" || control.WorkspaceID == "" || control.CapturedEvent == nil || control.CapturedEvent.SessionID != control.WorkspaceID || !isFileChange(*control.CapturedEvent) {
			return true, fmt.Errorf("invalid capture assignment")
		}
		id, err := strconv.ParseUint(control.CapturedEvent.ID, 10, 64)
		if err != nil || id == 0 {
			return true, fmt.Errorf("invalid captured revision ID")
		}
		// Metadata and the exact revision share one atomic JSON record.
		h.assigned[control.ObservationID] = control.WorkspaceID
		h.store(*control.CapturedEvent)
		if id > h.next {
			h.next = id
		}
		return true, nil
	}
	return false, nil
}

// Clear workspace claims without affecting ownership in other workflows.
func (h *Hub) clearClaimsLocked(id string) {
	for key := range h.claims {
		if strings.HasPrefix(key, id+"\n") {
			delete(h.claims, key)
		}
	}
}
