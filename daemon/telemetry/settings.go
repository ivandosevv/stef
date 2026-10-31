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
		return "report_event: major milestones only; reuse a brief public why. No per-file reports or polling. context_id answers explicit requests only."
	case UpdatesActivity:
		return "Activity: share brief public choices with action decided; no private thinking, prompts, commands, or raw tool output. " + fileReportingInstructions
	case UpdatesDisabled:
		return "Updates disabled. Stop reporting and check-ins for this turn; read the mode once at the next task."
	default:
		return "Files only; no extra choice reports. " + fileReportingInstructions
	}
}

const fileReportingInstructions = "After each edit: hook IDs or one get_state (changed_paths=own saves, settle_ms=300, limit=1), then report_changes before another edit/test/final. Reuse public text as common summary/why; override different reasons. Exact current-window IDs only. No polling or duplicate chat prose. context_id answers explicit requests only."
