package telemetry

// ChangeExplanation attaches a supplied reason to one observed disk revision.
type ChangeExplanation struct {
	ChangeID string   `json:"change_id" jsonschema:"whole-file observation ID from pending_changes"`
	Summary  string   `json:"summary" jsonschema:"short label containing one to five words"`
	Why      string   `json:"why" jsonschema:"brief actual reason for this exact change"`
	Parents  []string `json:"parents,omitempty" jsonschema:"related repository paths or symbols"`
}

type PendingChange struct {
	ChangeID string `json:"change_id"`
	NodeName string `json:"node_name"`
	Action   string `json:"action"`
}
