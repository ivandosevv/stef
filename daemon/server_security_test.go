package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ivandosevv/stef/daemon/hub"
)

func TestListenerAuthorityAndExactBrowserOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, host, origin string
		allowed            bool
	}{
		{"native client", "127.0.0.1:4820", "", true},
		{"same browser origin", "127.0.0.1:4820", "http://127.0.0.1:4820", true},
		{"localhost origin", "localhost:4820", "http://localhost:4820", true},
		{"IPv6 origin", "[::1]:4820", "http://[::1]:4820", true},
		{"wrong Host port", "127.0.0.1:5173", "", false},
		{"different local port", "127.0.0.1:4820", "http://127.0.0.1:5173", false},
		{"different local hostname", "127.0.0.1:4820", "http://localhost:4820", false},
		{"different loopback address", "127.0.0.1:4820", "http://127.0.0.2:4820", false},
		{"DNS rebinding", "untrusted.example:4820", "", false},
		{"userinfo Host", "evil@127.0.0.1:4820", "", false},
		{"origin path", "127.0.0.1:4820", "http://127.0.0.1:4820/", false},
		{"origin query", "127.0.0.1:4820", "http://127.0.0.1:4820?x", false},
		{"origin empty query", "127.0.0.1:4820", "http://127.0.0.1:4820?", false},
		{"origin fragment", "127.0.0.1:4820", "http://127.0.0.1:4820#x", false},
		{"origin empty fragment", "127.0.0.1:4820", "http://127.0.0.1:4820#", false},
		{"origin userinfo", "127.0.0.1:4820", "http://evil@127.0.0.1:4820", false},
		{"HTTPS origin", "127.0.0.1:4820", "https://127.0.0.1:4820", false},
		{"null origin", "127.0.0.1:4820", "null", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4820/api/events", nil)
			request.Host = tc.host
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			response := httptest.NewRecorder()
			localOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), "4820").ServeHTTP(response, request)
			want := http.StatusForbidden
			if tc.allowed {
				want = http.StatusNoContent
			}
			if response.Code != want {
				t.Fatalf("got %d, want %d: %s", response.Code, want, response.Body.String())
			}
		})
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4820/api/events", nil)
	request.Header.Add("Origin", "http://127.0.0.1:4820")
	request.Header.Add("Origin", "http://127.0.0.1:4820")
	response := httptest.NewRecorder()
	localOnly(http.NotFoundHandler(), "4820").ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatal("multiple Origin headers accepted")
	}
}

func TestListenerAuthorityUsesActualBoundPort(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4928/api/events", nil)
	request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4928}))
	serve := localOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), "4820")
	response := httptest.NewRecorder()
	serve.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatal("actual listener port was rejected")
	}
	request.Host = "127.0.0.1:4820"
	response = httptest.NewRecorder()
	serve.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatal("configured port overrode the actual listener")
	}
}

func TestFetchMetadataProtectsPrivateRoutesWithoutOrigin(t *testing.T) {
	for _, tc := range []struct {
		path, site, mode, destination string
		allowed                       bool
	}{
		{"/api/events", "same-origin", "cors", "empty", true},
		{"/api/events", "", "", "", true},
		{"/api/events", "cross-site", "cors", "empty", false},
		{"/api/events", "same-site", "cors", "empty", false},
		{"/ws", "cross-site", "websocket", "empty", false},
		{"/mcp", "cross-site", "cors", "empty", false},
		{"/api/events", "cross-site", "navigate", "document", false},
		{"/", "cross-site", "navigate", "iframe", true},
		{"/api/events", "cross-site", "navigate", "iframe", false},
		{"/", "cross-site", "navigate", "document", true},
	} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4820"+tc.path, nil)
		request.Header.Set("Sec-Fetch-Site", tc.site)
		request.Header.Set("Sec-Fetch-Mode", tc.mode)
		request.Header.Set("Sec-Fetch-Dest", tc.destination)
		response := httptest.NewRecorder()
		localOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), "4820").ServeHTTP(response, request)
		if (response.Code == http.StatusNoContent) != tc.allowed {
			t.Fatalf("unexpected metadata result %d for %+v", response.Code, tc)
		}
	}
}

func TestPrivateResponsesHaveSecurityHeaders(t *testing.T) {
	_, server := setup(t)
	response, err := http.Get(server.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	for name, expected := range map[string]string{"Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff"} {
		if response.Header.Get(name) != expected {
			t.Fatalf("missing %s", name)
		}
	}
	policy := response.Header.Get("Content-Security-Policy")
	for _, rule := range []string{"script-src 'self'", "object-src 'none'", "base-uri 'none'", "frame-ancestors 'self'"} {
		if !strings.Contains(policy, rule) {
			t.Fatalf("missing CSP directive %s", rule)
		}
	}
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (w *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func TestBodyDeadlinePreservesStreamingWritesAndKeepAlive(t *testing.T) {
	start := time.Now()
	response := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4820/mcp", strings.NewReader("{}"))
	localOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(response.deadlines) != 1 {
			t.Fatal("body was read before a deadline was set")
		}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: ready\n\n")
	}), "4820").ServeHTTP(response, request)
	if len(response.deadlines) != 2 || response.deadlines[0].Before(start.Add(4*time.Second)) || response.deadlines[0].After(start.Add(6*time.Second)) || !response.deadlines[1].IsZero() {
		t.Fatalf("finite body deadline not reset for keep-alive: %+v", response.deadlines)
	}
	if response.Body.String() != "data: ready\n\n" {
		t.Fatal("streamed output changed")
	}
}

func TestIncompleteBodiesCannotBlockResponse(t *testing.T) {
	_, server := setup(t)
	authority := strings.TrimPrefix(server.URL, "http://")
	conn, err := net.DialTimeout("tcp", authority, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(7 * time.Second))
	_, err = io.WriteString(conn, "POST /api/events HTTP/1.1\r\nHost: "+authority+"\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{")
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodPost})
	if err != nil {
		t.Fatalf("incomplete body was not rejected within its deadline: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected incomplete-body response: %d", response.StatusCode)
	}
}

func TestMCPBodyBudgetRejectsOversizedJSON(t *testing.T) {
	_, server := setup(t)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(strings.Repeat(" ", 1024*1024+1)+"{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("MCP body budget returned %d", response.StatusCode)
	}
}

func TestCompanionConnectionQuotaReleasesClosedRequests(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	serve := limitConnections(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	}), 1)
	run := func(response *httptest.ResponseRecorder) <-chan struct{} {
		done := make(chan struct{})
		go func() { serve.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ws", nil)); close(done) }()
		return done
	}
	first := httptest.NewRecorder()
	finished := run(first)
	<-entered
	second := httptest.NewRecorder()
	serve.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/ws", nil))
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") == "" {
		t.Fatal("excess companion connection accepted")
	}
	release <- struct{}{}
	<-finished
	third := httptest.NewRecorder()
	finished = run(third)
	<-entered
	release <- struct{}{}
	<-finished
	if third.Code != http.StatusNoContent {
		t.Fatal("closed request did not release its connection slot")
	}
}

func TestWebSocketSameOriginAndReadOnlyFrames(t *testing.T) {
	_, server := setup(t)
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	for _, origin := range []string{"http://localhost:5173", "http://untrusted.example", server.URL + "/", server.URL + "#"} {
		conn, response, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Origin": []string{origin}})
		if conn != nil {
			conn.Close()
		}
		if response != nil {
			response.Body.Close()
		}
		if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
			t.Fatalf("unrelated websocket origin accepted: %s", origin)
		}
	}
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, http.Header{"Origin": []string{server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var snapshot hub.Message
	if err := conn.ReadJSON(&snapshot); err != nil || snapshot.Type != "snapshot" {
		t.Fatalf("same-origin snapshot missing: %+v %v", snapshot, err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("unsupported")); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("unsupported client data was not closed: %v", err)
	}
}
