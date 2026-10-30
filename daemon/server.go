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

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/mcp"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func handler(h *hub.Hub, uiDir, port string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewHTTPHandler(h))
	mux.HandleFunc("GET /ws", h.ServeWS)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "name": "stef", "version": "0.3.0", "repo": h.Snapshot().Repo})
	})
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, h.Settings())
	})
	mux.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var input telemetry.Settings
		if !decodeJSON(w, r, &input, 1024) {
			return
		}
		if !input.UpdateMode.Valid() {
			http.Error(w, "update_mode must be milestones, files, or disabled", http.StatusBadRequest)
			return
		}
		settings, err := h.SetUpdateMode(input.UpdateMode)
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
		snapshot, err := h.ActivateSession(input.SessionID)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, 200, mcp.SessionReceipt{SessionID: snapshot.SessionID, UpdateMode: snapshot.UpdateMode, ReportingInstructions: snapshot.UpdateMode.ReportingInstructions()})
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

func localOnly(next http.Handler, port string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localHost(r.Host) {
			http.Error(w, "loopback Host required", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Scheme != "http" || !localHost(u.Host) || u.User != nil || (u.Port() != port && u.Port() != "5173") {
				http.Error(w, "origin is not an stef local UI", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
