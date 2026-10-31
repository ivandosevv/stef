package telemetry

// ChangeExplanation attaches a supplied reason to one observed disk revision.
type ChangeExplanation struct {
	ChangeID string   `json:"change_id" jsonschema:"whole-file observation ID from pending_changes"`
	Summary  string   `json:"summary,omitempty" jsonschema:"short label; omit to reuse the batch summary"`
	Why      string   `json:"why,omitempty" jsonschema:"actual reason for this exact change; omit to reuse the batch why only when it applies"`
	Parents  []string `json:"parents,omitempty" jsonschema:"related repository paths or symbols"`
}

type PendingChange struct {
	ChangeID string `json:"change_id"`
	NodeName string `json:"node_name"`
	Action   string `json:"action"`
}
