package setup

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"text/template"
)

const ClaudeSkillPath = ".claude/skills/stef/SKILL.md"

//go:embed claude_skill.md
var claudeSkillTemplate string

// Existing skills remain the user's source of truth, including root-confined
// links to a checkout's shared skill. Validate every component before setup
// writes either client configuration file.
func prepareClaudeSkill(root *os.Root, repo, executable, endpoint string) ([]byte, error) {
	if !filepath.IsAbs(executable) {
		return nil, fmt.Errorf("stef executable must be an absolute path")
	}
	for _, name := range []string{".claude", ".claude/skills", ".claude/skills/stef"} {
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			return renderClaudeSkill(repo, executable, endpoint)
		}
		if err != nil {
			return nil, fmt.Errorf("cannot inspect Claude skill path %s: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			info, err = root.Stat(name)
			if err != nil {
				return nil, fmt.Errorf("Claude skill link %s must resolve inside the project: %w", name, err)
			}
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("Claude skill path %s must be a directory", name)
		}
	}
	if _, err := root.Lstat(ClaudeSkillPath); err != nil {
		return nil, fmt.Errorf("existing stef skill needs a readable SKILL.md; review %s: %w", ClaudeSkillPath, err)
	}
	info, err := root.Stat(ClaudeSkillPath)
	if err != nil {
		return nil, fmt.Errorf("existing stef SKILL.md must resolve inside the project: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("existing stef SKILL.md must be a regular file")
	}
	raw, err := readOptional(root, ClaudeSkillPath)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("existing stef SKILL.md is empty; review %s", ClaudeSkillPath)
	}
	return nil, nil
}

func renderClaudeSkill(repo, executable, endpoint string) ([]byte, error) {
	tmpl, err := template.New("claude-stef").Parse(claudeSkillTemplate)
	if err != nil {
		return nil, err
	}
	data := struct {
		Root, Executable, Endpoint, EnableCommand, ReportCommand string
	}{
		Root:          strconv.Quote(repo),
		Executable:    strconv.Quote(executable),
		Endpoint:      strconv.Quote(endpoint),
		EnableCommand: shellQuote(executable) + " --enable --client claude --view ide --repo " + shellQuote(repo) + " --daemon " + shellQuote(endpoint),
		ReportCommand: shellQuote(executable) + " --report-changes --repo " + shellQuote(repo) + " --daemon " + shellQuote(endpoint),
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
