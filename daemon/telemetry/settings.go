package telemetry

// UpdateMode is a workspace reporting preference, independent of view appearance.
type UpdateMode string

const (
	UpdatesMilestones UpdateMode = "milestones"
	UpdatesFiles      UpdateMode = "files"
	UpdatesActivity   UpdateMode = "activity"
	UpdatesDisabled   UpdateMode = "disabled"
)

type Settings struct {
	UpdateMode UpdateMode `json:"update_mode"`
}

func (mode UpdateMode) Valid() bool {
	return mode == UpdatesMilestones || mode == UpdatesFiles || mode == UpdatesActivity || mode == UpdatesDisabled
}

// CapturesFiles includes exact revisions in both detailed reporting modes.
func (mode UpdateMode) CapturesFiles() bool {
	return mode == UpdatesFiles || mode == UpdatesActivity
}

// ReportingInstructions is deliberately compact: passive capture makes no model calls.
func (mode UpdateMode) ReportingInstructions() string {
	switch mode {
	case UpdatesMilestones:
		return "Report major milestones with a short why using report_event; no automatic per-file reasons. Answer explicitly requested context via report_event context_id."
	case UpdatesActivity:
		return "Capture supported tool activity locally; report concise choices with action decided and a brief user-facing why. Never share private reasoning transcripts, prompts, commands, or output. " + UpdatesFiles.ReportingInstructions()
	case UpdatesDisabled:
		return "Updates disabled. Stop reporting and check-ins for this turn; read the mode once at the next task."
	default:
		return "After each edit, use hook pending IDs or get_state once with changed_paths (actual own saved paths) and settle_ms=300; report_changes with each own exact reason before another edit, test, or final reply. Never backfill earlier windows or report extra decisions outside activity mode. context_id answers explicit requests only."
	}
}
