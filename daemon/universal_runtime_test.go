package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func testBinaryName() string {
	if runtime.GOOS == "windows" {
		return "stef.exe"
	}
	return "stef"
}

// Run the actual portable executable so a native-only startup assumption cannot
// hide behind a platform-simulated unit test or an explicit --headless flag.
func TestPortableBuildStartsFromAgentAndFallsBackFromNative(t *testing.T) {
	if testing.Short() {
		t.Skip("portable process integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	executable := filepath.Join(t.TempDir(), testBinaryName())
	build := exec.CommandContext(ctx, "go", "build", "-o", executable, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("portable build failed: %v: %s", err, output)
	}
	for _, view := range []string{"", "native"} {
		name := "default"
		if view != "" {
			name = view
		}
		t.Run(name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"--enable", "--client", "generic", "--repo", root, "--no-persist", "--quit-after", "2s"}
			if view != "" {
				args = append(args, "--view", view)
			}
			output, err := exec.CommandContext(ctx, executable, args...).Output()
			if err != nil {
				t.Fatalf("portable agent setup failed: %v", err)
			}
			var receipt struct {
				Repo             string `json:"repo"`
				Endpoint         string `json:"endpoint"`
				View             string `json:"view"`
				RequestedView    string `json:"requested_view"`
				NativeAvailable  bool   `json:"native_available"`
				ClientConfigured bool   `json:"client_configured"`
				Connected        bool   `json:"mcp_connected"`
			}
			if err := json.Unmarshal(output, &receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.Repo != root || receipt.View != "ide" || receipt.NativeAvailable || receipt.ClientConfigured || receipt.Connected || receipt.RequestedView != view {
				t.Fatalf("portable setup returned an incorrect presentation or connection claim: %s", output)
			}
			if ready, err := probeCompanion(ctx, receipt.Endpoint, root); err != nil || !ready {
				t.Fatalf("portable companion did not start beside its agent: %v", err)
			}
			if selected, err := projectPresentation(root, ""); err != nil || selected != "ide" {
				t.Fatalf("portable setup would resume into an unavailable native host: %s %v", selected, err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if ready, err := probeCompanion(ctx, receipt.Endpoint, root); err == nil && !ready {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Fatal("portable test companion outlived its requested lifetime")
		})
	}
}
