package hub

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session_id")
	if session != "" && telemetry.Session(session) != nil {
		http.Error(w, "invalid session_id", http.StatusBadRequest)
		return
	}
	// The enclosing HTTP middleware validates loopback authority and browser
	// metadata. Retain an origin check here if another host mounts this handler.
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096, CheckOrigin: websocketOrigin}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	var messages <-chan Message
	var unsubscribe func()
	if session != "" {
		messages, unsubscribe = h.SubscribeSession(session)
	} else {
		messages, unsubscribe = h.Subscribe()
	}
	defer unsubscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadLimit(1024)
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			// This connection sends snapshots only. Feedback has a bounded JSON
			// endpoint, so unsolicited data frames are neither needed nor replayed.
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "companion stream is read-only"), time.Now().Add(time.Second))
			return
		}
	}()
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

func websocketOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return true // Non-browser local clients do not send Origin.
	}
	if len(origins) != 1 {
		return false
	}
	u, err := url.Parse(origins[0])
	if err != nil || u.Scheme != "http" || origins[0] != "http://"+u.Host || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	request, err := url.Parse("http://" + r.Host)
	if err != nil {
		return false
	}
	port := func(u *url.URL) string {
		if u.Port() == "" {
			return "80"
		}
		return u.Port()
	}
	return strings.EqualFold(u.Hostname(), request.Hostname()) && port(u) == port(request)
}
