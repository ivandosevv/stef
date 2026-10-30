package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"sync"
)

type WorkspaceInput struct {
	SessionID   string `json:"session_id,omitempty" jsonschema:"origin session ID returned by set_session or get_state"`
	AgentID     string `json:"agent_id,omitempty" jsonschema:"this worker's stable unique reporter ID"`
	Action      string `json:"action" jsonschema:"create starts fresh sharing; join requires its name or ID; leave restores private history"`
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"existing shared workspace name or ID, required only for join"`
}
type WorkspaceReceipt struct {
	SessionID string `json:"session_id"`
	hub.Workspace
}

func workspaceDetails(h *hub.Hub, id string) hub.Workspace {
	for _, room := range h.Workspaces() {
		if room.WorkspaceID == id {
			return room
		}
	}
	return hub.Workspace{WorkspaceID: id, Members: []string{id}}
}

// Transport defaults are temporary. Stable chat/worker IDs select durable history.
// Separate reporters in one transport default to separate private workflows.
type identityBindings struct {
	mu      sync.Mutex
	workers map[*sdk.ServerSession]map[string]string
}

func newIdentityBindings() *identityBindings {
	return &identityBindings{workers: make(map[*sdk.ServerSession]map[string]string)}
}
func requestSession(req *sdk.CallToolRequest) *sdk.ServerSession {
	if req == nil {
		return nil
	}
	return req.Session
}
func validateWorker(agent string) error {
	if len(agent) > 80 || strings.TrimSpace(agent) != agent {
		return fmt.Errorf("agent_id must be at most 80 bytes without surrounding spaces")
	}
	return nil
}
func (ids *identityBindings) selectSession(req *sdk.CallToolRequest, input SessionInput) (string, error) {
	if err := telemetry.Session(input.SessionID); err != nil {
		return "", err
	}
	if err := validateWorker(input.AgentID); err != nil {
		return "", err
	}
	id := input.SessionID
	if input.AgentID != "" {
		sum := sha256.Sum256([]byte(input.SessionID + "\n" + input.AgentID))
		id = "chat-" + hex.EncodeToString(sum[:16])
	}
	ids.mu.Lock()
	defer ids.mu.Unlock()
	key := requestSession(req)
	if ids.workers[key] == nil {
		ids.workers[key] = map[string]string{}
	}
	ids.workers[key][input.AgentID] = id
	return id, nil
}
func (ids *identityBindings) resolve(ctx context.Context, b backend, req *sdk.CallToolRequest, explicit, agent string) (string, error) {
	if err := validateWorker(agent); err != nil {
		return "", err
	}
	if explicit != "" {
		if err := telemetry.Session(explicit); err != nil {
			return "", err
		}
		if _, err := b.activate(ctx, explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	ids.mu.Lock()
	defer ids.mu.Unlock()
	key := requestSession(req)
	if ids.workers[key] == nil {
		ids.workers[key] = map[string]string{}
	}
	id := ids.workers[key][agent]
	if id == "" {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", err
		}
		id = "agent-" + hex.EncodeToString(random[:])
	}
	if _, err := b.activate(ctx, id); err != nil {
		return "", err
	}
	ids.workers[key][agent] = id
	return id, nil
}
