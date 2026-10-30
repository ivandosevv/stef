// Package mcp exposes the same small telemetry contract over stdio and HTTP.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	ChangeID  string   `json:"change_id,omitempty" jsonschema:"optional exact whole-file observation ID"`
	ContextID string   `json:"context_id,omitempty" jsonschema:"retained visible step ID; only to answer developer-requested extra context"`
	SessionID string   `json:"session_id,omitempty" jsonschema:"chat ID returned by set_session; omit only when hooks select the chat"`
	AgentID   string   `json:"agent_id" jsonschema:"stable reporter ID"`
	NodeName  string   `json:"node_name" jsonschema:"relative path or path#symbol"`
	Action    string   `json:"action" jsonschema:"scouted, created, modified, deleted, tested"`
	Summary   string   `json:"summary" jsonschema:"one to five words"`
	Why       string   `json:"why" jsonschema:"one short sentence explaining the decision"`
	Status    string   `json:"status,omitempty" jsonschema:"running or ready"`
	Parents   []string `json:"parents,omitempty" jsonschema:"related paths or symbols"`
}

// Receipt deliberately excludes diffs, code, and repeated rationale from the model response.
type Receipt struct {
	UpdateMode            telemetry.UpdateMode      `json:"update_mode"`
	UpdatesFrom           string                    `json:"updates_from"`
	ReportingInstructions string                    `json:"reporting_instructions"`
	ID                    string                    `json:"id"`
	Accepted              bool                      `json:"accepted"`
	Collisions            []string                  `json:"collisions,omitempty"`
	Feedback              []telemetry.FeedbackNote  `json:"feedback,omitempty"`
	PendingChanges        []telemetry.PendingChange `json:"pending_changes,omitempty"`
	PendingCount          int                       `json:"pending_count"`
}
type ChangesInput struct {
	SessionID string                        `json:"session_id,omitempty" jsonschema:"chat ID; defaults to the active chat"`
	AgentID   string                        `json:"agent_id" jsonschema:"stable reporter ID"`
	Changes   []telemetry.ChangeExplanation `json:"changes" jsonschema:"one to fifty exact file-change explanations"`
}
type ChangesReceipt struct {
	UpdateMode            telemetry.UpdateMode      `json:"update_mode"`
	UpdatesFrom           string                    `json:"updates_from"`
	ReportingInstructions string                    `json:"reporting_instructions"`
	Accepted              bool                      `json:"accepted"`
	IDs                   []string                  `json:"ids"`
	Feedback              []telemetry.FeedbackNote  `json:"feedback,omitempty"`
	PendingChanges        []telemetry.PendingChange `json:"pending_changes,omitempty"`
	PendingCount          int                       `json:"pending_count"`
}
type StateInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"chat ID; defaults to the active chat"`
	Limit     int    `json:"limit,omitempty" jsonschema:"recent events, default 10, maximum 50"`
	AgentID   string `json:"agent_id,omitempty" jsonschema:"optional stable reporter ID to receive pending developer notes at this check-in"`
	SettleMS  int    `json:"settle_ms,omitempty" jsonschema:"optional bounded delay after editing for watcher debounce, zero to 1000 milliseconds"`
}
type StateEvent struct {
	AgentID  string `json:"agent_id"`
	NodeName string `json:"node_name"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
	Phase    string `json:"phase,omitempty"`
}
type State struct {
	UpdateMode            telemetry.UpdateMode      `json:"update_mode"`
	UpdatesFrom           string                    `json:"updates_from"`
	ReportingInstructions string                    `json:"reporting_instructions"`
	SessionID             string                    `json:"session_id"`
	Repo                  string                    `json:"repo"`
	Events                []StateEvent              `json:"events"`
	Feedback              []telemetry.FeedbackNote  `json:"feedback,omitempty"`
	PendingChanges        []telemetry.PendingChange `json:"pending_changes"`
	PendingCount          int                       `json:"pending_count"`
}
type Publication struct {
	UpdateMode            telemetry.UpdateMode      `json:"update_mode"`
	UpdatesFrom           string                    `json:"updates_from"`
	ReportingInstructions string                    `json:"reporting_instructions"`
	Accepted              bool                      `json:"accepted"`
	Event                 telemetry.Event           `json:"event"`
	Feedback              []telemetry.FeedbackNote  `json:"feedback,omitempty"`
	PendingChanges        []telemetry.PendingChange `json:"pending_changes,omitempty"`
	PendingCount          int                       `json:"pending_count"`
}
type backend interface {
	publish(context.Context, telemetry.Event) (Publication, error)
	state(context.Context, StateInput) (State, error)
	changes(context.Context, ChangesInput) (ChangesReceipt, error)
	activate(context.Context, string) (SessionReceipt, error)
}
type SessionInput struct {
	SessionID string `json:"session_id" jsonschema:"stable ID for this chat; reuse the same ID when resuming it"`
}
type SessionReceipt struct {
	SessionID             string               `json:"session_id"`
	UpdateMode            telemetry.UpdateMode `json:"update_mode"`
	UpdatesFrom           string               `json:"updates_from"`
	ReportingInstructions string               `json:"reporting_instructions"`
}
type localBackend struct{ hub *hub.Hub }

func (b localBackend) publish(_ context.Context, e telemetry.Event) (Publication, error) {
	e.Source = "agent"
	stored, err := b.hub.Publish(e)
	if err != nil {
		return Publication{}, err
	}
	return Published(b.hub, stored), nil
}
func (b localBackend) state(ctx context.Context, input StateInput) (State, error) {
	return CheckInRequest(ctx, b.hub, input)
}
func (b localBackend) changes(_ context.Context, input ChangesInput) (ChangesReceipt, error) {
	return ReportChanges(b.hub, input)
}
func (b localBackend) activate(_ context.Context, id string) (SessionReceipt, error) {
	snapshot, err := b.hub.ActivateSession(id)
	return SessionReceipt{SessionID: snapshot.SessionID, UpdateMode: snapshot.UpdateMode, UpdatesFrom: snapshot.UpdatesFrom, ReportingInstructions: snapshot.UpdateMode.ReportingInstructions()}, err
}
func CheckIn(h *hub.Hub, limit int, agent string, session ...string) State {
	id := ""
	if len(session) > 0 {
		id = session[0]
	}
	s := h.SessionSnapshot(id)
	if s.UpdateMode == telemetry.UpdatesDisabled {
		return State{SessionID: s.SessionID, Repo: s.Repo, UpdateMode: s.UpdateMode, UpdatesFrom: s.UpdatesFrom, ReportingInstructions: s.UpdateMode.ReportingInstructions(), Events: []StateEvent{}, PendingChanges: []telemetry.PendingChange{}}
	}
	visible := make([]telemetry.Event, 0, len(s.Events))
	from, _ := strconv.ParseUint(s.UpdatesFrom, 10, 64)
	for _, event := range s.Events {
		id, err := strconv.ParseUint(event.ID, 10, 64)
		if err != nil || id < from || event.ContextID != "" {
			continue
		}
		if s.UpdateMode == telemetry.UpdatesMilestones && (event.Source != "agent" || event.ChangeID != "") {
			continue
		}
		visible = append(visible, event)
	}
	state := compact(s.Repo, visible, limit)
	state.SessionID = s.SessionID
	state.UpdateMode = s.UpdateMode
	state.UpdatesFrom = s.UpdatesFrom
	state.ReportingInstructions = s.UpdateMode.ReportingInstructions()
	state.Feedback = h.OfferFeedback(agent, s.SessionID)
	state.PendingChanges = []telemetry.PendingChange{}
	if s.UpdateMode == telemetry.UpdatesFiles {
		state.PendingChanges, state.PendingCount = h.PendingChanges(s.SessionID, 50)
	}
	return state
}
func compact(repo string, events []telemetry.Event, limit int) State {
	filtered := make([]telemetry.Event, 0, len(events))
	for _, e := range events {
		if e.Category != "feedback" {
			filtered = append(filtered, e)
		}
	}
	events = filtered
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	state := State{Repo: repo, Events: make([]StateEvent, 0, len(events))}
	for _, e := range events {
		state.Events = append(state.Events, StateEvent{AgentID: e.AgentID, NodeName: e.NodeName, Summary: e.Summary, Status: e.Status, Phase: e.Phase})
	}
	return state
}
func newServer(b backend) *sdk.Server {
	no := false
	server := sdk.NewServer(&sdk.Implementation{Name: "stef", Version: "0.3.0"}, &sdk.ServerOptions{Instructions: "stef is connected. At task start, get_state once and follow update_mode and reporting_instructions in receipts. milestones: use report_event only for major steps with a brief why; no per-file reporting. files: after EACH edit tool, get_state once with settle_ms=300 and explain every own pending change_id in one report_changes batch, including helpers, entry points, tests, docs, and cooperating agents' edits. A later revision needs its own reason; check pending changes once before finishing. updates_from is the inclusive first event ID of the current reporting window. Mode changes never request older saved-file explanations; omit pre-window IDs from automatic report_changes batches. disabled: stop reporting and extra checks for this turn; read the mode again on the next task. Mode changes reach a working agent at its next check-in, so never poll. Design choices render local evidence and do not increase reporting volume. Only when a developer explicitly requests more context, answer with report_event context_id matching their exact retained ID, node_name, and action; this is allowed in milestones but not disabled. Do not generate extra context automatically; do not combine context_id with change_id. Use 2–4 word labels and a short user-facing why (about 12 words). Only explain edits attributable to your task. Consider advisory developer notes at natural breaks. No second model calls or hidden reasoning. Trusted hooks select chat IDs; otherwise set_session once for a new/resumed chat and retain its ID across turns. Use parents only for actual relationships; other arrows show sequence."})
	sdk.AddTool(server, &sdk.Tool{Name: "set_session", Description: "Select this chat's workflow once on new chat or resume. New IDs start empty; existing IDs restore saved steps."}, func(ctx context.Context, _ *sdk.CallToolRequest, input SessionInput) (*sdk.CallToolResult, SessionReceipt, error) {
		if err := telemetry.Session(input.SessionID); err != nil {
			return nil, SessionReceipt{}, err
		}
		result, err := b.activate(ctx, input.SessionID)
		return nil, result, err
	})
	sdk.AddTool(server, &sdk.Tool{Name: "report_event", Description: "Share a meaningful workflow step as work starts; briefly update its result.", Annotations: &sdk.ToolAnnotations{DestructiveHint: &no, OpenWorldHint: &no}}, func(ctx context.Context, _ *sdk.CallToolRequest, input Input) (*sdk.CallToolResult, Receipt, error) {
		event, err := telemetry.Normalize(telemetry.Event{SessionID: input.SessionID, AgentID: input.AgentID, ChangeID: input.ChangeID, ContextID: input.ContextID, NodeName: input.NodeName, Action: input.Action, Summary: input.Summary, Why: input.Why, Status: input.Status, Parents: input.Parents})
		if err != nil {
			return nil, Receipt{}, err
		}
		stored, err := b.publish(ctx, event)
		if err != nil {
			return nil, Receipt{}, err
		}
		return nil, Receipt{ID: stored.Event.ID, Accepted: stored.Accepted, UpdateMode: stored.UpdateMode, UpdatesFrom: stored.UpdatesFrom, ReportingInstructions: stored.ReportingInstructions, Collisions: stored.Event.Collisions, Feedback: stored.Feedback, PendingChanges: stored.PendingChanges, PendingCount: stored.PendingCount}, nil
	})
	sdk.AddTool(server, &sdk.Tool{Name: "report_changes", Description: "Explain every file revision changed by an edit in one batch. Use exact change_id values from pending_changes; do not omit entry points, tests, or docs.", Annotations: &sdk.ToolAnnotations{DestructiveHint: &no, OpenWorldHint: &no}}, func(ctx context.Context, _ *sdk.CallToolRequest, input ChangesInput) (*sdk.CallToolResult, ChangesReceipt, error) {
		result, err := b.changes(ctx, input)
		return nil, result, err
	})
	sdk.AddTool(server, &sdk.Tool{Name: "get_state", Description: "Check compact recent activity when coordination is needed. Supply agent_id to receive pending developer notes once; do not poll.", Annotations: &sdk.ToolAnnotations{DestructiveHint: &no, OpenWorldHint: &no}}, func(ctx context.Context, _ *sdk.CallToolRequest, input StateInput) (*sdk.CallToolResult, State, error) {
		limit := input.Limit
		if limit == 0 {
			limit = 10
		}
		if limit < 1 || limit > 50 {
			return nil, State{}, fmt.Errorf("limit must be 1–50")
		}
		if len(input.AgentID) > 80 {
			return nil, State{}, fmt.Errorf("agent_id must be at most 80 bytes")
		}
		if input.SessionID != "" {
			if err := telemetry.Session(input.SessionID); err != nil {
				return nil, State{}, err
			}
		}
		input.Limit = limit
		input.AgentID = strings.TrimSpace(input.AgentID)
		if input.SettleMS < 0 || input.SettleMS > 1000 {
			return nil, State{}, fmt.Errorf("settle_ms must be 0–1000")
		}
		state, err := b.state(ctx, input)
		return nil, state, err
	})
	return server
}
func NewLocalServer(h *hub.Hub) *sdk.Server { return newServer(localBackend{hub: h}) }
func NewHTTPHandler(h *hub.Hub) http.Handler {
	server := NewLocalServer(h)
	return sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
}

type remoteBackend struct {
	base   string
	client *http.Client
}

func (b remoteBackend) publish(ctx context.Context, event telemetry.Event) (Publication, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return Publication{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+"/api/events", bytes.NewReader(data))
	if err != nil {
		return Publication{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return Publication{}, fmt.Errorf("stef companion unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 1000))
		return Publication{}, fmt.Errorf("daemon rejected event: %s", message)
	}
	output := Publication{UpdatesFrom: "0"}
	err = json.NewDecoder(response.Body).Decode(&output)
	return output, err
}
func (b remoteBackend) state(ctx context.Context, input StateInput) (State, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return State{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+"/api/check-in", bytes.NewReader(data))
	if err != nil {
		return State{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return State{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return State{}, fmt.Errorf("daemon returned HTTP %d", response.StatusCode)
	}
	state := State{UpdatesFrom: "0"}
	if err := json.NewDecoder(io.LimitReader(response.Body, 128*1024)).Decode(&state); err != nil {
		return State{}, err
	}
	return state, nil
}
func (b remoteBackend) changes(ctx context.Context, input ChangesInput) (ChangesReceipt, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return ChangesReceipt{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+"/api/changes", bytes.NewReader(data))
	if err != nil {
		return ChangesReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return ChangesReceipt{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 1000))
		return ChangesReceipt{}, fmt.Errorf("daemon rejected explanations: %s", message)
	}
	receipt := ChangesReceipt{UpdatesFrom: "0"}
	err = json.NewDecoder(io.LimitReader(response.Body, 128*1024)).Decode(&receipt)
	return receipt, err
}
func (b remoteBackend) activate(ctx context.Context, id string) (SessionReceipt, error) {
	data, _ := json.Marshal(SessionInput{SessionID: id})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+"/api/sessions/activate", bytes.NewReader(data))
	if err != nil {
		return SessionReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return SessionReceipt{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return SessionReceipt{}, fmt.Errorf("could not select chat (HTTP %d)", response.StatusCode)
	}
	result := SessionReceipt{UpdatesFrom: "0"}
	err = json.NewDecoder(io.LimitReader(response.Body, 1024)).Decode(&result)
	return result, err
}
func ValidateEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("daemon URL must be an HTTP loopback origin")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("daemon URL must use localhost or a loopback IP")
	}
	return nil
}
func NewServer(endpoint string) (*sdk.Server, error) {
	if err := ValidateEndpoint(endpoint); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return newServer(remoteBackend{base: strings.TrimSuffix(endpoint, "/"), client: client}), nil
}
func Run(ctx context.Context, endpoint string) error {
	server, err := NewServer(endpoint)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}
