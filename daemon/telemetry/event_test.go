package telemetry_test

import (
	"github.com/ivandosevv/stef/daemon/telemetry"
	"strings"
	"testing"
)

func TestValidation(t *testing.T) {
	valid := telemetry.Event{AgentID: "agent", NodeName: "src/main.go#Run", Action: "modified", Summary: "Added request validation", Why: "Requests need a clear boundary."}
	cases := []struct {
		name   string
		change func(*telemetry.Event)
	}{
		{"traversal", func(e *telemetry.Event) { e.NodeName = "src/../../outside.go" }},
		{"absolute", func(e *telemetry.Event) { e.NodeName = "/tmp/file.go" }},
		{"windows traversal", func(e *telemetry.Event) { e.NodeName = "..\\secret.go" }},
		{"six words", func(e *telemetry.Event) { e.Summary = "one two three four five six" }},
		{"empty why", func(e *telemetry.Event) { e.Why = " " }},
		{"invalid status", func(e *telemetry.Event) { e.Status = "collision_warning" }},
		{"invalid action", func(e *telemetry.Event) { e.Action = "executed" }},
		{"oversize code", func(e *telemetry.Event) { e.After = strings.Repeat("a", telemetry.MaxText+1) }},
		{"outside parent", func(e *telemetry.Event) { e.Parents = []string{"../outside"} }},
		{"invalid change ID", func(e *telemetry.Event) { e.ChangeID = "some-file" }},
		{"zero change ID", func(e *telemetry.Event) { e.ChangeID = "0" }},
		{"noncanonical change ID", func(e *telemetry.Event) { e.ChangeID = "01" }},
		{"oversized change ID", func(e *telemetry.Event) { e.ChangeID = "99999999999999999999" }},
		{"invalid context ID", func(e *telemetry.Event) { e.ContextID = "file" }},
		{"zero context ID", func(e *telemetry.Event) { e.ContextID = "0" }},
		{"noncanonical context ID", func(e *telemetry.Event) { e.ContextID = "01" }},
		{"oversized context ID", func(e *telemetry.Event) { e.ContextID = "99999999999999999999" }},
		{"combined explanation IDs", func(e *telemetry.Event) { e.ChangeID = "1"; e.ContextID = "2" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := valid
			tc.change(&e)
			if _, err := telemetry.Normalize(e); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	valid.Summary = "  Added   request validation  "
	normalized, err := telemetry.Normalize(valid)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Status != "running" || normalized.Summary != "Added request validation" {
		t.Fatalf("unexpected normalized event: %+v", normalized)
	}
}
