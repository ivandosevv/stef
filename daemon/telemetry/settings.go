package telemetry

// UpdateMode is a project-wide reporting preference, independent of chat history.
type UpdateMode string

const (
	UpdatesMilestones UpdateMode = "milestones"
	UpdatesFiles      UpdateMode = "files"
	UpdatesDisabled   UpdateMode = "disabled"
)

type Settings struct {
	UpdateMode UpdateMode `json:"update_mode"`
}

func (mode UpdateMode) Valid() bool {
	return mode == UpdatesMilestones || mode == UpdatesFiles || mode == UpdatesDisabled
}

// ReportingInstructions is deliberately compact: passive capture makes no model calls.
func (mode UpdateMode) ReportingInstructions() string {
	switch mode {
	case UpdatesMilestones:
		return "Report major milestones with a short why using report_event; no automatic per-file reasons. Answer explicitly requested context via report_event context_id."
	case UpdatesDisabled:
		return "Updates disabled. Stop reporting; check the mode once at the next turn or natural break."
	default:
		return "After each edit, get_state once with settle_ms=300; explain every own pending change_id in one report_changes batch. Never backfill earlier reporting windows. Use context_id only for requested extra context."
	}
}
