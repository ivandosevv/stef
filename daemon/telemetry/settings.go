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
		return "Updates disabled. Stop reporting and check-ins for this turn; read the mode once at the next task."
	default:
		return "After each edit, use hook pending IDs or get_state once with settle_ms=300; report_changes with each own exact reason before another edit, test, or final reply. Never backfill earlier windows. context_id answers explicit requests only."
	}
}
