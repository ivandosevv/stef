package telemetry

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ReminderLimit = 50

// ReminderRequest names the files attributable to a concrete edit tool.
type ReminderRequest struct {
	SessionID string   `json:"session_id,omitempty"`
	Paths     []string `json:"paths"`
	SettleMS  int      `json:"settle_ms,omitempty"`
}

// ReminderState carries revision IDs only, without code, feedback, or rationale.
type ReminderState struct {
	Repo           string          `json:"repo"`
	SessionID      string          `json:"session_id"`
	UpdateMode     UpdateMode      `json:"update_mode"`
	UpdatesFrom    string          `json:"updates_from"`
	PendingChanges []PendingChange `json:"pending_changes"`
	PendingCount   int             `json:"pending_count"`
}

func (r ReminderRequest) Validate() error {
	if r.SessionID != "" {
		if err := Session(r.SessionID); err != nil {
			return err
		}
	}
	if r.SettleMS < 0 || r.SettleMS > 1000 {
		return fmt.Errorf("settle_ms must be 0–1000")
	}
	_, err := ReminderPaths(r.Paths)
	return err
}

// ReminderPaths rejects symbols, virtual nodes, and noncanonical paths.
// Duplicate file paths are harmless; at most fifty distinct files are accepted.
func ReminderPaths(paths []string) (map[string]bool, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("paths must name 1–50 distinct repository-relative files")
	}
	files := make(map[string]bool)
	for _, value := range paths {
		node, err := Node(value)
		if err != nil || node != value || value == "@workflow" || strings.Contains(value, "#") || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("paths must contain plain canonical repository-relative files")
		}
		files[value] = true
		if len(files) > ReminderLimit {
			return nil, fmt.Errorf("paths must name at most 50 distinct files")
		}
	}
	return files, nil
}
