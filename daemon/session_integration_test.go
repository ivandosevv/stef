package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	bridge "github.com/ivandosevv/stef/daemon/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestChatSelectionAndFeedbackOverBothMCPTransports(t *testing.T) {
	for _, transport := range []string{"forwarding", "http"} {
		t.Run(transport, func(t *testing.T) {
			_, server := setup(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var connection sdk.Transport
			if transport == "http" {
				connection = &sdk.StreamableClientTransport{Endpoint: server.URL + "/mcp"}
			} else {
				mcpServer, err := bridge.NewServer(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				peer, local := sdk.NewInMemoryTransports()
				serverSession, err := mcpServer.Connect(ctx, peer, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer serverSession.Close()
				connection = local
			}
			client := sdk.NewClient(&sdk.Implementation{Name: "chat-test", Version: "1"}, nil)
			session, err := client.Connect(ctx, connection, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			call := func(tool string, arguments map[string]any, output any) {
				t.Helper()
				result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: arguments})
				if err != nil || result.IsError {
					t.Fatalf("%s failed: %+v %v", tool, result, err)
				}
				raw, _ := json.Marshal(result.StructuredContent)
				if err := json.Unmarshal(raw, output); err != nil {
					t.Fatal(err)
				}
			}
			var receipt bridge.SessionReceipt
			call("set_session", map[string]any{"session_id": "chat-a", "agent_id": "same-agent"}, &receipt)
			if receipt.SessionID == "" {
				t.Fatal("chat identity missing from MCP receipt")
			}
			firstID := receipt.SessionID
			var publication bridge.Receipt
			call("report_event", map[string]any{"session_id": firstID, "agent_id": "same-agent", "node_name": "a.go", "action": "modified", "summary": "Fix blanks", "why": "Blank names need a default."}, &publication)
			note, _ := json.Marshal(map[string]string{"session_id": firstID, "kind": "suggestion", "text": "Check Unicode too."})
			response, err := http.Post(server.URL+"/api/feedback", "application/json", bytes.NewReader(note))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 201 {
				t.Fatal("note rejected")
			}
			call("set_session", map[string]any{"session_id": "chat-b", "agent_id": "same-agent"}, &receipt)
			var state bridge.State
			call("get_state", map[string]any{"agent_id": "same-agent"}, &state)
			if state.SessionID != receipt.SessionID || state.SessionID == firstID || len(state.Events) != 0 || len(state.Feedback) != 0 {
				t.Fatalf("new chat inherited data: %+v", state)
			}
			call("set_session", map[string]any{"session_id": "chat-a", "agent_id": "same-agent"}, &receipt)
			call("get_state", map[string]any{"agent_id": "same-agent"}, &state)
			if state.SessionID != firstID || len(state.Events) != 1 || len(state.Feedback) != 1 {
				t.Fatalf("resume lost data: %+v", state)
			}
			clearBody, _ := json.Marshal(map[string]string{"session_id": firstID})
			response, err = http.Post(server.URL+"/api/sessions/clear", "application/json", bytes.NewReader(clearBody))
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatal("clear rejected")
			}
			state = bridge.State{}
			call("get_state", map[string]any{"agent_id": "same-agent"}, &state)
			if len(state.Events) != 0 || len(state.Feedback) != 0 {
				t.Fatal("clear left workflow data behind")
			}
		})
	}
}
