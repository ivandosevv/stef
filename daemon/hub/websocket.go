package hub

import (
	"net/http"
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
	// Origin and Host are checked by the enclosing HTTP middleware.
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096, CheckOrigin: func(*http.Request) bool { return true }}
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
