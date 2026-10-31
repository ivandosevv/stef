package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/telemetry"
)

func Published(h *hub.Hub, stored telemetry.Event) Publication {
	state := CheckIn(h, 1, stored.AgentID, stored.SessionID)
	return Publication{Event: stored, Accepted: stored.ID != "", UpdateMode: state.UpdateMode, UpdatesFrom: state.UpdatesFrom, ReportingInstructions: state.ReportingInstructions, Feedback: state.Feedback, PendingChanges: state.PendingChanges, PendingCount: state.PendingCount}
}

func ReportChanges(h *hub.Hub, input ChangesInput) (ChangesReceipt, error) {
	// Reuse supplied public text without making another model call or mutating
	// the caller's batch. The hub still validates each exact revision atomically.
	changes := make([]telemetry.ChangeExplanation, len(input.Changes))
	for i, change := range input.Changes {
		if change.Summary == "" {
			change.Summary = input.Summary
		}
		if change.Why == "" {
			change.Why = input.Why
		}
		changes[i] = change
	}
	events, err := h.ExplainChanges(input.SessionID, input.AgentID, changes)
	if err != nil {
		return ChangesReceipt{}, err
	}
	state := CheckIn(h, 1, input.AgentID, input.SessionID)
	receipt := ChangesReceipt{Accepted: len(events) > 0, IDs: make([]string, len(events)), UpdateMode: state.UpdateMode, UpdatesFrom: state.UpdatesFrom, ReportingInstructions: state.ReportingInstructions, Feedback: state.Feedback, PendingChanges: state.PendingChanges, PendingCount: state.PendingCount}
	for i, event := range events {
		receipt.IDs[i] = event.ID
	}
	return receipt, nil
}

// CheckInRequest performs one bounded debounce wait, never polling or making model calls.
func CheckInRequest(ctx context.Context, h *hub.Hub, input StateInput) (State, error) {
	if input.Limit == 0 {
		input.Limit = 10
	}
	if input.Limit < 1 || input.Limit > 50 || len(input.AgentID) > 80 {
		return State{}, fmt.Errorf("invalid check-in limit or agent_id")
	}
	if input.SessionID != "" {
		if err := telemetry.Session(input.SessionID); err != nil {
			return State{}, err
		}
	}
	if input.SettleMS < 0 || input.SettleMS > 1000 {
		return State{}, fmt.Errorf("settle_ms must be 0–1000")
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	if input.SettleMS > 0 {
		timer := time.NewTimer(time.Duration(input.SettleMS) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return State{}, ctx.Err()
		case <-timer.C:
		}
	}
	if len(input.ChangedPaths) > 0 {
		if err := h.ClaimChanges(input.SessionID, input.ChangedPaths); err != nil {
			return State{}, err
		}
	}
	return CheckIn(h, input.Limit, strings.TrimSpace(input.AgentID), input.SessionID), nil
}
