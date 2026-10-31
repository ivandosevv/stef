package main

import (
	"encoding/json"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func handler(h *hub.Hub, uiDir, port string) http.Handler {
	mux := http.NewServeMux()
	mcpHandler := mcp.NewHTTPHandler(h)
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		mcpHandler.ServeHTTP(w, r)
	}))
	mux.Handle("GET /ws", limitConnections(http.HandlerFunc(h.ServeWS), 64))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "name": "stef", "version": "0.3.0", "repo": h.Snapshot().Repo})
	})
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, h.Settings(r.URL.Query().Get("session_id")))
	})
	mux.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			telemetry.Settings
			SessionID string `json:"session_id,omitempty"`
		}
		if !decodeJSON(w, r, &input, 1024) {
			return
		}
		if !input.UpdateMode.Valid() {
			http.Error(w, "update_mode must be milestones, files, or disabled", http.StatusBadRequest)
			return
		}
		scope := r.URL.Query().Get("session_id")
		if scope != "" && input.SessionID != "" && scope != input.SessionID {
			http.Error(w, "conflicting session_id", 400)
			return
		}
		if scope == "" {
			scope = input.SessionID
		}
		if scope != "" && telemetry.Session(scope) != nil {
			http.Error(w, "invalid session_id", http.StatusBadRequest)
			return
		}
		settings, err := h.SetUpdateMode(input.UpdateMode, scope)
		if err != nil {
			http.Error(w, "cannot persist update mode", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
	mux.HandleFunc("POST /api/sessions/activate", func(w http.ResponseWriter, r *http.Request) {
		var input mcp.SessionInput
		if !decodeJSON(w, r, &input, 1024) {
			return
		}
		receipt, err := mcp.ActivateSession(h, input.SessionID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, receipt)
	})
	mux.HandleFunc("GET /api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": h.Workspaces(), "current_workspace_id": h.Snapshot().SessionID})
	})
	mux.HandleFunc("POST /api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		var input hub.WorkspaceInput
		if !decodeJSON(w, r, &input, 4096) {
			return
		}
		workspace, err := h.ConfigureWorkspace(input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, workspace)
	})
	mux.HandleFunc("POST /api/sessions/clear", func(w http.ResponseWriter, r *http.Request) {
		var input mcp.SessionInput
		if !decodeJSON(w, r, &input, 1024) {
			return
		}
		snapshot, err := h.ClearSession(input.SessionID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, snapshot)
	})
	mux.HandleFunc("POST /api/feedback", func(w http.ResponseWriter, r *http.Request) {
		var note telemetry.Feedback
		if !decodeJSON(w, r, &note, 4096) {
			return
		}
		event, err := note.Event()
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		stored, err := h.Publish(event)
		if err != nil {
			http.Error(w, "cannot persist developer note", 500)
			return
		}
		writeJSON(w, 201, map[string]string{"id": stored.ID})
	})
	mux.HandleFunc("POST /api/check-in", func(w http.ResponseWriter, r *http.Request) {
		var input mcp.StateInput
		if !decodeJSON(w, r, &input, 4096) {
			return
		}
		state, err := mcp.CheckInRequest(r.Context(), h, input)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, state)
	})
	mux.HandleFunc("POST /api/changes", func(w http.ResponseWriter, r *http.Request) {
		var input mcp.ChangesInput
		if !decodeJSON(w, r, &input, 1024*1024) {
			return
		}
		receipt, err := mcp.ReportChanges(h, input)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 201, receipt)
	})
	mux.HandleFunc("POST /api/explanation-reminder", func(w http.ResponseWriter, r *http.Request) {
		var input telemetry.ReminderRequest
		if !decodeJSON(w, r, &input, 32*1024) {
			return
		}
		if err := input.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if input.SettleMS > 0 {
			timer := time.NewTimer(time.Duration(input.SettleMS) * time.Millisecond)
			defer timer.Stop()
			select {
			case <-r.Context().Done():
				http.Error(w, r.Context().Err().Error(), http.StatusBadRequest)
				return
			case <-timer.C:
			}
		}
		if err := r.Context().Err(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.ClaimChanges(input.SessionID, input.Paths); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		state, err := h.ExplanationReminder(input.SessionID, input.Paths)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, state)
	})
	mux.HandleFunc("POST /api/activity", func(w http.ResponseWriter, r *http.Request) {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			http.Error(w, "Content-Type must be application/json", 415)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var activity telemetry.Activity
		if err := decoder.Decode(&activity); err != nil {
			http.Error(w, "invalid activity JSON", 400)
			return
		}
		if decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "expected one JSON object", 400)
			return
		}
		event, err := activity.Event()
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		// A subagent may first appear in a tool callback rather than SessionStart.
		if activity.SessionID != "" {
			if _, err := h.RegisterSession(activity.SessionID); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
		}
		stored, err := h.Publish(event)
		if err != nil {
			http.Error(w, "cannot persist activity", 500)
			return
		}
		writeJSON(w, 201, map[string]string{"id": stored.ID})
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		session := r.URL.Query().Get("session_id")
		if session != "" && telemetry.Session(session) != nil {
			http.Error(w, "invalid session_id", 400)
			return
		}
		snapshot := h.SessionSnapshot(session)
		if raw := r.URL.Query().Get("limit"); raw != "" {
			limit, err := strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > hub.HistoryLimit {
				http.Error(w, "limit must be between 1 and 5000", http.StatusBadRequest)
				return
			}
			if len(snapshot.Events) > limit {
				snapshot.Events = snapshot.Events[len(snapshot.Events)-limit:]
			}
		}
		writeJSON(w, http.StatusOK, snapshot)
	})
	mux.HandleFunc("POST /api/events", func(w http.ResponseWriter, r *http.Request) {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var event telemetry.Event
		if err := decoder.Decode(&event); err != nil {
			http.Error(w, "invalid event JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "expected one JSON object", http.StatusBadRequest)
			return
		}
		event.Source = "agent"
		event.Category = ""
		event.Phase = ""
		event.Tool = ""
		event.NoteKind = ""
		event.TargetAgent = ""
		stored, err := h.Publish(event)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, mcp.Published(h, stored))
	})
	if _, err := os.Stat(filepath.Join(uiDir, "index.html")); uiDir != "" && err == nil {
		mux.Handle("/", http.FileServer(http.Dir(uiDir)))
	} else if assets, err := fs.Sub(bundledUI, "assets"); err == nil {
		if _, err := fs.Stat(assets, "index.html"); err == nil {
			mux.Handle("/", http.FileServer(http.FS(assets)))
		} else {
			mux.HandleFunc("/", missingUI)
		}
	} else {
		mux.HandleFunc("/", missingUI)
	}
	return localOnly(mux, port)
}

func missingUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><title>stef</title><h1>stef engine is running</h1><p>Build the companion with <code>make build</code>.</p>`)
}
func decodeJSON(w http.ResponseWriter, r *http.Request, value any, limit int64) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		http.Error(w, "Content-Type must be application/json", 415)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "invalid JSON", 400)
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "expected one JSON object", 400)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func localHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// requestAuthority validates the HTTP origin syntax without DNS resolution.
// Cookies and loopback addresses alone do not distinguish local web origins.
func requestAuthority(authority string) (string, string, bool) {
	u, err := url.Parse("http://" + authority)
	if err != nil || u.Host != authority || strings.ContainsAny(authority, "/\\?#@") || strings.HasSuffix(authority, ":") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || !localHost(u.Host) {
		return "", "", false
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
		return "", "", false
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), port), port, true
}

func sameOrigin(origin, authority string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || origin != "http://"+u.Host || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	value, _, ok := requestAuthority(u.Host)
	return ok && value == authority
}

// A WebSocket occupies its own quota until closed. Finite HTTP requests and MCP
// streams use separate paths so idle companion tabs cannot starve agent reports.
func limitConnections(next http.Handler, maximum int) http.Handler {
	active := make(chan struct{}, maximum)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case active <- struct{}{}:
			defer func() { <-active }()
		default:
			w.Header().Set("Retry-After", "2")
			http.Error(w, "too many companion connections", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type finiteRequestBody struct {
	io.ReadCloser
	controller *http.ResponseController
}

func (body *finiteRequestBody) Read(p []byte) (int, error) {
	n, err := body.ReadCloser.Read(p)
	if err == io.EOF {
		_ = body.controller.SetReadDeadline(time.Time{})
	}
	return n, err
}

func localOnly(next http.Handler, port string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authority, requestPort, ok := requestAuthority(r.Host)
		listenPort := port
		if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
			if _, actual, err := net.SplitHostPort(local.String()); err == nil {
				listenPort = actual
			}
		}
		if !ok || requestPort != listenPort {
			http.Error(w, "loopback listener Host required", http.StatusForbidden)
			return
		}
		if origins := r.Header.Values("Origin"); len(origins) > 0 {
			if len(origins) != 1 || !sameOrigin(origins[0], authority) {
				http.Error(w, "same-origin companion required", http.StatusForbidden)
				return
			}
		}
		// Browser metadata also covers GET requests that omit Origin. Initial
		// top-level navigation can load the public shell; it contains no history.
		site := r.Header.Get("Sec-Fetch-Site")
		destination := r.Header.Get("Sec-Fetch-Dest")
		navigation := r.Method == http.MethodGet && r.URL.Path == "/" && r.Header.Get("Sec-Fetch-Mode") == "navigate" && (destination == "document" || destination == "iframe")
		if site != "" && site != "same-origin" && site != "none" && !navigation {
			http.Error(w, "cross-origin browser request rejected", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self' vscode-webview: vscode-file:")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp" || r.URL.Path == "/healthz" {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method == http.MethodPost {
			// Bound body reads without imposing a write deadline on MCP SSE.
			controller := http.NewResponseController(w)
			if controller.SetReadDeadline(time.Now().Add(5*time.Second)) == nil {
				// Clear only after EOF. Clearing when a handler rejects incomplete
				// JSON would leave net/http draining that body without a deadline.
				r.Body = &finiteRequestBody{ReadCloser: r.Body, controller: controller}
			}
		}
		next.ServeHTTP(w, r)
	})
}
