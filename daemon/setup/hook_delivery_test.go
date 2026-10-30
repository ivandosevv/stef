package setup

import (
	"encoding/json"
	"testing"
)

func TestPostEditHookDeliveryMigratesOnlyStefAndIsIdempotent(t *testing.T) {
	legacy := []byte(`{"description":"keep","hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"custom","async":true},{"type":"command","command":"stef-hook","async":true,"timeout":9}]}]}}`)
	updated, err := configureHookEvents(legacy, "stef-hook", []string{"PostToolUse"}, false)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Description string
		Hooks       map[string][]struct {
			Hooks []struct {
				Command string
				Async   bool
				Timeout int
			}
		}
	}
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatal(err)
	}
	handlers := document.Hooks["PostToolUse"][0].Hooks
	if document.Description != "keep" || len(document.Hooks["PostToolUse"]) != 1 || len(handlers) != 2 || !handlers[0].Async || handlers[1].Async || handlers[1].Timeout != 9 {
		t.Fatal("post-edit migration duplicated stef or changed unrelated settings")
	}
	again, err := configureHookEvents(updated, "stef-hook", []string{"PostToolUse"}, false)
	if err != nil || string(again) != string(updated) {
		t.Fatal("post-edit migration was not idempotent")
	}
}

func TestPostEditHookIsSynchronousForBothClients(t *testing.T) {
	for _, syncStop := range []bool{false, true} {
		raw, err := configureHookEvents(nil, "stef-hook", []string{"PreToolUse", "PostToolUse", "Stop"}, syncStop)
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Hooks map[string][]struct {
				Hooks []struct {
					Async   bool
					Timeout int
				}
			}
		}
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		if !document.Hooks["PreToolUse"][0].Hooks[0].Async || document.Hooks["PostToolUse"][0].Hooks[0].Async || document.Hooks["PostToolUse"][0].Hooks[0].Timeout != 3 || document.Hooks["Stop"][0].Hooks[0].Async == syncStop {
			t.Fatal("post-edit context can arrive after the model resumes or other lifecycle timing changed")
		}
	}
}
