package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestClosedMCPTransportReleasesBindingsKeepsHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	server := sdk.NewServer(&sdk.Implementation{Name: "binding-test", Version: "1"}, nil)
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	ids := newIdentityBindings()
	request := &sdk.CallToolRequest{Session: serverSession}
	input := SessionInput{SessionID: "durable-chat", AgentID: "worker"}
	origin, err := ids.selectSession(request, input)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := ids.resolve(ctx, localBackend{hub: h}, request, "", "worker")
	if err != nil || resolved != origin {
		t.Fatalf("bound identity changed: %s %v", resolved, err)
	}
	if _, err := h.Publish(telemetry.Event{SessionID: origin, AgentID: "worker", NodeName: "main.go", Action: "modified", Summary: "Preserved history", Why: "Transport cleanup must preserve recorded work.", Status: "ready", Source: "agent"}); err != nil {
		t.Fatal(err)
	}
	if err := clientSession.Close(); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		ids.mu.Lock()
		_, retained := ids.workers[serverSession]
		ids.mu.Unlock()
		if !retained {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("closed transport retained worker bindings")
		case <-ticker.C:
		}
	}
	if snapshot := h.SessionSnapshot(origin); len(snapshot.Events) != 1 {
		t.Fatalf("transport cleanup removed durable history: %+v", snapshot)
	}
	resumed, err := ids.selectSession(nil, input)
	if err != nil || resumed != origin {
		t.Fatalf("resumed chat identity changed: %s %v", resumed, err)
	}
}

func TestNilRequestsRetainBindingsWithoutSessionWatchers(t *testing.T) {
	ids := newIdentityBindings()
	for i := 0; i < 10; i++ {
		if _, err := ids.selectSession(nil, SessionInput{SessionID: "durable-chat", AgentID: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	ids.mu.Lock()
	defer ids.mu.Unlock()
	if len(ids.workers) != 1 || len(ids.workers[nil]) != 1 {
		t.Fatalf("nil request created transport state: %+v", ids.workers)
	}
}

func TestMCPIdleTransportExpires(t *testing.T) {
	if transportIdleTimeout != 30*time.Minute {
		t.Fatal("production transport idle budget changed")
	}
	h, err := hub.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	idle := 100 * time.Millisecond
	server := httptest.NewServer(newHTTPHandler(h, idle))
	defer server.Close()
	call := func(session, method string, params map[string]any) *http.Response {
		t.Helper()
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
		if err != nil {
			t.Fatal(err)
		}
		request, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			request.Header.Set("Mcp-Session-Id", session)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := call("", "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "idle-test", "version": "1"}})
	session := response.Header.Get("Mcp-Session-Id")
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || session == "" {
		t.Fatalf("test transport did not initialize: %d %q", response.StatusCode, session)
	}
	time.Sleep(3 * idle)
	response = call(session, "tools/list", map[string]any{})
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("abandoned transport remained active: %d", response.StatusCode)
	}
}

func TestMCPStreamQuotaDoesNotConsumeToolRequestSlots(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	serve := limitHTTPRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- r.Method
		<-release
		w.WriteHeader(http.StatusNoContent)
	}), 1)
	run := func(method string) (*httptest.ResponseRecorder, <-chan struct{}) {
		response := httptest.NewRecorder()
		done := make(chan struct{})
		go func() { serve.ServeHTTP(response, httptest.NewRequest(method, "/mcp", nil)); close(done) }()
		return response, done
	}
	_, streamDone := run(http.MethodGet)
	<-entered
	request, requestDone := run(http.MethodPost)
	<-entered
	blocked := httptest.NewRecorder()
	serve.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatal("excess tool request was accepted")
	}
	release <- struct{}{}
	release <- struct{}{}
	<-streamDone
	<-requestDone
	if request.Code != http.StatusNoContent {
		t.Fatal("stream occupied the tool request quota")
	}
	close(release)
	response := httptest.NewRecorder()
	serve.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if response.Code != http.StatusNoContent {
		t.Fatal("closed request did not release its slot")
	}
}
