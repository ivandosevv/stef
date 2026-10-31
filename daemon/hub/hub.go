// Package hub maintains an ordered, durable event stream and nonblocking subscribers.
package hub

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

const HistoryLimit = 5000
const ClaimTTL = 30 * time.Second

type Message struct {
	Type        string               `json:"type"`
	SessionID   string               `json:"session_id"`
	Repo        string               `json:"repo,omitempty"`
	UpdateMode  telemetry.UpdateMode `json:"update_mode"`
	UpdatesFrom string               `json:"updates_from"`
	Events      []telemetry.Event    `json:"events,omitempty"`
	Event       *telemetry.Event     `json:"event,omitempty"`
	WorkspaceID string               `json:"workspace_id,omitempty"`
	Workspaces  []Workspace          `json:"workspaces,omitempty"`
}

type Hub struct {
	mu                sync.Mutex
	repo              string
	events            []telemetry.Event
	active            string
	sessions          map[string][]telemetry.Event
	next              uint64
	clients           map[chan Message]struct{}
	claims            map[string]map[string]time.Time
	journal           *os.File
	offered           map[string]map[string]bool
	updateMode        telemetry.UpdateMode
	updatesFrom       uint64
	captureEpoch      uint64
	workspaceEnabled  bool
	memberships       map[string]string
	shared            map[string]bool
	workspaceSettings map[string]workspaceSetting
	workspaceNames    map[string]string
	originFrom        map[string]uint64
	assigned          map[string]string
	scopedClients     map[chan Message]string
}

func New(repo, journalPath string) (*Hub, error) {
	h := &Hub{repo: repo, active: "local", sessions: map[string][]telemetry.Event{"local": {}}, events: make([]telemetry.Event, 0), clients: make(map[chan Message]struct{}), claims: make(map[string]map[string]time.Time), offered: make(map[string]map[string]bool), updateMode: telemetry.UpdatesFiles, memberships: make(map[string]string), shared: make(map[string]bool), workspaceSettings: make(map[string]workspaceSetting), workspaceNames: make(map[string]string), originFrom: make(map[string]uint64), assigned: make(map[string]string), scopedClients: make(map[chan Message]string)}
	if journalPath == "" {
		if err := h.migrateWorkspaceNamesLocked(); err != nil {
			return nil, err
		}
		return h, nil
	}
	file, err := openJournal(repo, journalPath)
	if err != nil {
		return nil, fmt.Errorf("open event journal: %w", err)
	}
	// A partial last line from an interrupted write is truncated on recovery.
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), MaxJournalRecord)
	var offset int64
	for scanner.Scan() {
		raw := scanner.Bytes()
		var control sessionControl
		if err := json.Unmarshal(raw, &control); err == nil && control.Control != "" {
			if handled, err := h.replayWorkspaceControl(control); handled {
				if err != nil {
					file.Close()
					return nil, err
				}
				offset += int64(len(raw) + 1)
				continue
			}
			if control.Control == "settings" {
				if !control.UpdateMode.Valid() || (control.SessionID != "" && telemetry.Session(control.SessionID) != nil) {
					file.Close()
					return nil, fmt.Errorf("invalid settings journal record")
				}
				if control.SessionID != "" {
					setting := h.settingLocked(control.SessionID)
					if setting.Mode != control.UpdateMode {
						setting.Mode = control.UpdateMode
						setting.From = h.next + 1
						setting.CaptureFrom = h.next + 1
					}
					h.workspaceSettings[control.SessionID] = setting
				} else if h.updateMode != control.UpdateMode {
					h.updatesFrom = h.next + 1
					h.updateMode = control.UpdateMode
				}
				offset += int64(len(raw) + 1)
				continue
			}
			if telemetry.Session(control.SessionID) != nil || (control.Control != "activate" && control.Control != "clear") {
				file.Close()
				return nil, fmt.Errorf("invalid session journal record")
			}
			offset += int64(len(raw) + 1)
			if control.Control == "activate" {
				h.activate(control.SessionID)
			} else {
				h.sessions[control.SessionID] = []telemetry.Event{}
				if setting, ok := h.workspaceSettings[control.SessionID]; ok {
					setting.CaptureFrom = h.next + 1
					h.workspaceSettings[control.SessionID] = setting
				}
				if h.active == control.SessionID {
					h.events = h.sessions[control.SessionID]
				}
			}
			continue
		}
		var event telemetry.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			if err := file.Truncate(offset); err != nil {
				file.Close()
				return nil, err
			}
			break
		}
		offset += int64(len(raw) + 1)
		if event.SessionID == "" {
			event.SessionID = "local"
		}
		if err := h.admitWorkspaceLocked(event.SessionID); err != nil {
			file.Close()
			return nil, err
		}
		h.store(event)
		if id, err := strconv.ParseUint(event.ID, 10, 64); err == nil && id > h.next {
			h.next = id
		}
	}
	if err := scanner.Err(); err != nil {
		file.Close()
		return nil, fmt.Errorf("read journal: %w", err)
	}
	end, err := file.Seek(0, 2)
	if err != nil {
		file.Close()
		return nil, err
	}
	if offset > end {
		// Scanner accepts a valid JSON record at EOF even without its newline.
		if _, err := file.WriteString("\n"); err != nil {
			file.Close()
			return nil, err
		}
	}
	h.journal = file
	if err := h.migrateWorkspaceNamesLocked(); err != nil {
		file.Close()
		return nil, err
	}
	return h, nil
}

func (h *Hub) Publish(input telemetry.Event) (telemetry.Event, error) {
	e, err := telemetry.Normalize(input)
	if err != nil {
		return e, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if e.SessionID == "" {
		if e.Source == "watcher" && h.workspaceEnabled {
			e.SessionID = "local"
		} else {
			e.SessionID = h.active
		}
	}
	e.SessionID = h.resolveLocked(e.SessionID)
	if e.Source == "hook" && e.Phase == "started" && e.SessionID != h.active {
		if err := h.activateLocked(e.SessionID); err != nil {
			return e, err
		}
	}
	if h.suppressLocked(e) {
		e.ID = ""
		return e, nil
	}
	if err := h.validateChangeLocked(e); err != nil {
		return e, err
	}
	if err := h.validateContextLocked(e); err != nil {
		return e, err
	}
	if err := h.ensureWorkspaceNameLocked(e.SessionID); err != nil {
		return e, err
	}
	now := time.Now().UTC()
	e.ID = strconv.FormatUint(h.next+1, 10)
	e.Timestamp = now.Format(time.RFC3339Nano)
	if e.Source == "" {
		e.Source = "agent"
	}
	// File observation is not an agent claim. Claims expire or are explicitly released.
	if e.Source == "agent" && e.ContextID == "" {
		for node, owners := range h.claims {
			for agent, touched := range owners {
				if now.Sub(touched) > ClaimTTL {
					delete(owners, agent)
				}
			}
			if len(owners) == 0 {
				delete(h.claims, node)
			}
		}
		file, _, _ := strings.Cut(e.NodeName, "#")
		file = e.SessionID + "\n" + file
		if e.Status == "running" && (e.Action == "created" || e.Action == "modified") {
			owners := h.claims[file]
			if owners == nil {
				owners = make(map[string]time.Time)
				h.claims[file] = owners
			}
			for agent := range owners {
				if agent != e.AgentID {
					e.Collisions = append(e.Collisions, agent)
				}
			}
			sort.Strings(e.Collisions)
		}
	}
	data, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	if err := h.persist(data); err != nil {
		return e, err
	}
	if e.Source == "agent" && e.ContextID == "" {
		file, _, _ := strings.Cut(e.NodeName, "#")
		file = e.SessionID + "\n" + file
		if e.Status == "ready" || e.Action == "deleted" {
			delete(h.claims[file], e.AgentID)
		} else if e.Action == "created" || e.Action == "modified" {
			h.claims[file][e.AgentID] = now
		}
	}
	h.next++
	h.store(e)
	h.broadcast(h.eventMessageLocked(e))
	return e, nil
}

func (h *Hub) Snapshot() Message { h.mu.Lock(); defer h.mu.Unlock(); return h.snapshot() }
func (h *Hub) snapshot() Message {
	return h.snapshotSessionLocked(h.active)
}

func (h *Hub) Subscribe() (<-chan Message, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	client := make(chan Message, 128)
	client <- h.snapshot()
	h.clients[client] = struct{}{}
	return client, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.clients[client]; ok {
			delete(h.clients, client)
			close(client)
		}
	}
}

func (h *Hub) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		close(client)
		delete(h.clients, client)
	}
	for client := range h.scopedClients {
		close(client)
		delete(h.scopedClients, client)
	}
	if h.journal != nil {
		err := h.journal.Close()
		h.journal = nil
		return err
	}
	return nil
}
