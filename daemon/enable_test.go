package main

import (
	"path/filepath"
	"testing"

	clientsetup "github.com/ivandosevv/stef/daemon/setup"
)

func TestEnableReceiptDistinguishesImportsFromConnections(t *testing.T) {
	for _, configured := range []bool{true, false} {
		receipt := enableResult(clientsetup.Configuration{Client: "jetbrains", ConfigPath: ".stef/clients/jetbrains.json", Configured: configured, NextStep: "Import and approve the server."}, "/personal/project", "http://127.0.0.1:4837/", "ide")
		if receipt["client_configured"] != configured || receipt["mcp_connected"] != false || receipt["view_opened"] != false {
			t.Fatalf("setup made an unsupported active connection claim: %+v", receipt)
		}
		if receipt["configuration_path"] != filepath.Join("/personal/project", ".stef/clients/jetbrains.json") {
			t.Fatal("import path lost")
		}
		if _, ok := receipt["open_command"]; ok {
			t.Fatal("invented a VS Code command for JetBrains")
		}
	}
}
