package watcher

import (
	"regexp"
	"strings"
)

// Local credentials stay excluded even when Git is absent or tracks the file.
func sensitivePath(rel string) bool {
	for _, part := range strings.Split(strings.ToLower(rel), "/") {
		switch part {
		case ".ssh", ".aws", ".azure", ".kube", ".gnupg", ".config",
			".codex", ".claude", ".cursor", ".gemini", ".roo", ".devin", ".vscode",
			".npmrc", ".netrc", ".pypirc", ".git-credentials", ".mcp.json",
			"credentials", "credentials.json", "credentials.yaml", "credentials.yml",
			"opencode.json", "opencode.jsonc", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519":
			return true
		}
		if part == ".env" || strings.HasPrefix(part, ".env.") {
			return true
		}
		for _, suffix := range []string{".pem", ".key", ".p12", ".pfx", ".jks", ".keystore"} {
			if strings.HasSuffix(part, suffix) {
				return true
			}
		}
	}
	return false
}

// Conservative provider/key markers are local checks, never a model request.
// They reduce accidental capture; they are not a guarantee that arbitrary
// secrets can be recognized in source. Suppress the entire revision so exact
// diffs never silently substitute redacted text.
var credentialMarker = regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----|\b(?:AKIA|ASIA)[A-Z0-9]{16}\b|\bgh[pousr]_[A-Za-z0-9]{36,255}\b|\bgithub_pat_[A-Za-z0-9_]{80,255}\b|\bsk-(?:proj-|svcacct-|ant-)?[A-Za-z0-9_-]{32,255}\b|\bxox[baprs]-[A-Za-z0-9-]{24,255}\b`)

func sensitiveContent(content []byte) bool { return credentialMarker.Match(content) }
