package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ivandosevv/stef/daemon/telemetry"
)

func TestForwardingPreservesLegacyModeAndDefaultsOnlyMissingBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/events" || r.URL.Path == "/api/changes" {
			w.WriteHeader(http.StatusCreated)
		}
		_, _ = w.Write([]byte(`{"update_mode":"milestones","session_id":"chat-a"}`))
	}))
	defer server.Close()
	b := remoteBackend{base: server.URL, client: server.Client()}
	ctx := context.Background()
	check := func(mode telemetry.UpdateMode, from string, err error) {
		t.Helper()
		if err != nil || mode != telemetry.UpdatesMilestones || from != "0" {
			t.Fatalf("legacy forwarding changed mode or boundary: %q %q %v", mode, from, err)
		}
	}
	state, err := b.state(ctx, StateInput{})
	check(state.UpdateMode, state.UpdatesFrom, err)
	published, err := b.publish(ctx, telemetry.Event{})
	check(published.UpdateMode, published.UpdatesFrom, err)
	changes, err := b.changes(ctx, ChangesInput{})
	check(changes.UpdateMode, changes.UpdatesFrom, err)
	selected, err := b.activate(ctx, "chat-a")
	check(selected.UpdateMode, selected.UpdatesFrom, err)
}
