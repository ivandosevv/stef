package hub

import (
	"github.com/ivandosevv/stef/daemon/telemetry"
	"testing"
)

func TestFeedbackTargetingBatchingAndOncePerAgent(t *testing.T) {
	h, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, target := range []string{"alpha", "*", "alpha", "alpha", "beta"} {
		e, err := (telemetry.Feedback{TargetAgent: target, Kind: "question", Text: "Does this handle retries?"}).Event()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.Publish(e); err != nil {
			t.Fatal(err)
		}
	}
	if notes := h.OfferFeedback(""); len(notes) != 0 {
		t.Fatal("anonymous state consumed notes")
	}
	first := h.OfferFeedback("alpha")
	if len(first) != 3 || first[0].ID != "1" || first[1].ID != "2" || first[2].ID != "3" {
		t.Fatalf("wrong first batch: %+v", first)
	}
	if notes := h.OfferFeedback("alpha"); len(notes) != 1 || notes[0].ID != "4" {
		t.Fatalf("wrong second batch: %+v", notes)
	}
	if notes := h.OfferFeedback("alpha"); len(notes) != 0 {
		t.Fatal("notes were offered repeatedly")
	}
	if notes := h.OfferFeedback("beta"); len(notes) != 2 || notes[0].ID != "2" || notes[1].ID != "5" {
		t.Fatalf("targeting failed: %+v", notes)
	}
	if len(h.Snapshot().Events) != 5 {
		t.Fatal("offering deleted history")
	}
}
