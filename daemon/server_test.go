package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ivandosevv/stef/daemon/hub"
	bridge "github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func setup(t *testing.T) (*hub.Hub, *httptest.Server) {
	t.Helper()
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler(h, t.TempDir(), "4820"))
	t.Cleanup(func() { server.Close(); h.Close() })
	return h, server
}

func TestDeveloperNotesDeliveredAtMCPCheckInAcrossTransports(t *testing.T) {
	for _, transport := range []string{"stdio-forwarding", "streamable-http"} {
		t.Run(transport, func(t *testing.T) {
			h, server := setup(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client := sdk.NewClient(&sdk.Implementation{Name: "notes-test", Version: "1"}, nil)
			var connection sdk.Transport
			if transport == "streamable-http" {
				connection = &sdk.StreamableClientTransport{Endpoint: server.URL + "/mcp"}
			} else {
				bridgeServer, err := bridge.NewServer(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				serverTransport, clientTransport := sdk.NewInMemoryTransports()
				serverSession, err := bridgeServer.Connect(ctx, serverTransport, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer serverSession.Close()
				connection = clientTransport
			}
			session, err := client.Connect(ctx, connection, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			origins := map[string]string{}
			for _, agent := range []string{"alpha", "beta"} {
				result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_state", Arguments: map[string]any{"agent_id": agent}})
				if err != nil || result.IsError {
					t.Fatalf("private check-in failed: %+v %v", result, err)
				}
				data, _ := json.Marshal(result.StructuredContent)
				var state bridge.State
				if err := json.Unmarshal(data, &state); err != nil {
					t.Fatal(err)
				}
				origins[agent] = state.SessionID
			}
			if origins["alpha"] == origins["beta"] {
				t.Fatal("private reporter identities collided")
			}
			post := func(target string) {
				body, _ := json.Marshal(map[string]string{"session_id": origins[target], "target_agent": target, "kind": "concern", "text": "Check retry behavior before finishing.", "node_name": "main.go#Run"})
				response, err := http.Post(server.URL+"/api/feedback", "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != 201 {
					t.Fatal("note rejected")
				}
			}
			post("alpha")
			var arguments map[string]any
			_ = json.Unmarshal([]byte(validJSON()), &arguments)
			call := func(agent string) bridge.Receipt {
				arguments["agent_id"] = agent
				arguments["session_id"] = origins[agent]
				result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "report_event", Arguments: arguments})
				if err != nil || result.IsError {
					t.Fatalf("call failed: %+v %v", result, err)
				}
				data, _ := json.Marshal(result.StructuredContent)
				var receipt bridge.Receipt
				if err := json.Unmarshal(data, &receipt); err != nil {
					t.Fatal(err)
				}
				return receipt
			}
			if len(call("beta").Feedback) != 0 {
				t.Fatal("note leaked to a different agent")
			}
			if notes := call("alpha").Feedback; len(notes) != 1 || notes[0].Text != "Check retry behavior before finishing." {
				t.Fatalf("note missing: %+v", notes)
			}
			if len(call("alpha").Feedback) != 0 {
				t.Fatal("note delivered repeatedly")
			}
			post("alpha")
			result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_state", Arguments: map[string]any{"agent_id": "alpha"}})
			if err != nil || result.IsError {
				t.Fatalf("check-in failed: %+v %v", result, err)
			}
			data, _ := json.Marshal(result.StructuredContent)
			var state bridge.State
			_ = json.Unmarshal(data, &state)
			if len(state.Feedback) != 1 || len(state.Events) != 2 {
				t.Fatalf("state includes feedback as activity or misses note: %+v", state)
			}
			if len(h.OfferFeedback("alpha", origins["alpha"])) != 0 {
				t.Fatal("check-in did not mark note offered")
			}
		})
	}
}

func TestDeveloperNotesValidateAndDoNotConsumeOnHistoryRead(t *testing.T) {
	h, server := setup(t)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"kind":"question","text":"","target_agent":"alpha"}`, 400},
		{`{"kind":"command","text":"Do something"}`, 400},
		{`{"kind":"question","text":"Is this okay?","node_name":"../outside"}`, 400},
		{`{"kind":"question","text":"Is this okay?","extra":1}`, 400},
		{`{"kind":"question","text":"Should we check retries?","target_agent":"alpha"}`, 201},
	} {
		response, err := http.Post(server.URL+"/api/feedback", "application/json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatalf("got %d for %s", response.StatusCode, tc.body)
		}
	}
	response, err := http.Get(server.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(h.OfferFeedback("alpha")) != 1 {
		t.Fatal("reading history consumed feedback")
	}
	e, err := (telemetry.Feedback{Kind: "question", Text: strings.Repeat("Ж", 500)}).Event()
	if err != nil {
		t.Fatal("500-character Unicode note rejected", err)
	}
	if _, err := h.Publish(e); err != nil {
		t.Fatal(err)
	}
	if _, err := (telemetry.Feedback{Kind: "question", Text: strings.Repeat("x", 501)}).Event(); err == nil {
		t.Fatal("oversized note accepted")
	}
}
func validJSON() string {
	return "{\"agent_id\":\"agent\",\"node_name\":\"main.go#Run\",\"action\":\"modified\",\"summary\":\"Added validation\",\"why\":\"Requests need a clear boundary.\",\"status\":\"ready\"}"
}

func TestHTTPPublishesToWebSocketAndReconnectSnapshot(t *testing.T) {
	_, server := setup(t)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var snapshot hub.Message
	if err := conn.ReadJSON(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Type != "snapshot" {
		t.Fatal("missing snapshot")
	}
	response, err := http.Post(server.URL+"/api/events", "application/json", strings.NewReader(validJSON()))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("post returned %d", response.StatusCode)
	}
	var event hub.Message
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event.Event == nil || event.Event.Source != "agent" {
		t.Fatal("event not broadcast")
	}
	fresh, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	_ = fresh.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := fresh.ReadJSON(&snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Events) != 1 {
		t.Fatal("reconnect lost event history")
	}
}

func TestRejectsInvalidRequestsAndRemoteOrigins(t *testing.T) {
	h, _ := setup(t)
	route := handler(h, t.TempDir(), "4820")
	cases := []struct {
		name, body, origin, host, contentType string
		status                                int
	}{
		{"remote origin", validJSON(), "https://untrusted.example", "127.0.0.1:4820", "application/json", 403},
		{"rebinding host", validJSON(), "", "untrusted.example:4820", "application/json", 403},
		{"unapproved local port", validJSON(), "http://127.0.0.1:9999", "127.0.0.1:4820", "application/json", 403},
		{"form request", validJSON(), "", "127.0.0.1:4820", "text/plain", 415},
		{"multiple objects", validJSON() + validJSON(), "", "127.0.0.1:4820", "application/json", 400},
		{"oversize request", strings.Repeat(" ", 128*1024) + validJSON(), "", "127.0.0.1:4820", "application/json", 400},
		{"invalid path", strings.ReplaceAll(validJSON(), "main.go#Run", "../outside.go"), "", "127.0.0.1:4820", "application/json", 400},
		{"allowed dev origin", validJSON(), "http://localhost:5173", "127.0.0.1:4820", "application/json", 201},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "http://"+tc.host+"/api/events", strings.NewReader(tc.body))
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Content-Type", tc.contentType)
			response := httptest.NewRecorder()
			route.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}

func TestOfficialMCPHandshakeToolsAndForwarding(t *testing.T) {
	h, httpServer := setup(t)
	server, err := bridge.NewServer(httpServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	session, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "test-agent", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 5 {
		t.Fatalf("expected telemetry, state, and workspace tools: %+v", tools)
	}
	toolNames := map[string]bool{}
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"set_session", "set_workspace", "get_state", "report_event", "report_changes"} {
		if !toolNames[name] {
			t.Fatalf("missing MCP tool %s", name)
		}
	}
	var arguments map[string]any
	if err := json.NewDecoder(bytes.NewBufferString(validJSON())).Decode(&arguments); err != nil {
		t.Fatal(err)
	}
	result, err := clientSession.CallTool(ctx, &sdk.CallToolParams{Name: "report_event", Arguments: arguments})
	if err != nil || result.IsError {
		t.Fatalf("MCP call failed: %+v %v", result, err)
	}
	if len(h.Snapshot().Events) != 1 {
		t.Fatal("MCP event did not reach daemon")
	}
	arguments["summary"] = "one two three four five six"
	result, err = clientSession.CallTool(ctx, &sdk.CallToolParams{Name: "report_event", Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("MCP should return a tool error for invalid badge")
	}
	result, err = clientSession.CallTool(ctx, &sdk.CallToolParams{Name: "get_state", Arguments: map[string]any{"agent_id": "agent"}})
	if err != nil || result.IsError {
		t.Fatal("cannot read this reporter workspace")
	}
}

func TestStreamableMCPAndSmallReceipts(t *testing.T) {
	h, server := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "http-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var arguments map[string]any
	if err := json.Unmarshal([]byte(validJSON()), &arguments); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "report_event", Arguments: arguments})
	if err != nil || result.IsError {
		t.Fatalf("MCP HTTP call failed: %+v %v", result, err)
	}
	if len(h.Snapshot().Events) != 1 {
		t.Fatal("MCP HTTP event missing")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("Requests need a clear boundary")) || bytes.Contains(encoded, []byte("node_name")) {
		t.Fatal("receipt echoed the full event to the agent")
	}
	if !bytes.Contains(encoded, []byte("accepted")) {
		t.Fatal("missing acknowledgement")
	}
	response, err := http.Post(server.URL+"/api/activity", "application/json", strings.NewReader(`{"agent_id":"hook-agent","phase":"testing","tool":"test-runner"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatal("activity was rejected")
	}
	snapshot := h.Snapshot()
	last := snapshot.Events[len(snapshot.Events)-1]
	if last.Source != "hook" || last.Category != "activity" || last.Phase != "testing" {
		t.Fatal("activity was misrepresented as a decision")
	}
}
