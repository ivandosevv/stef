package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ivandosevv/stef/daemon/mcp"
)

// probeCompanion refuses to attach to a service watching a different repository.
func probeCompanion(ctx context.Context, endpoint, root string) (bool, error) {
	client := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(endpoint, "/")+"/healthz", nil)
	if err != nil {
		return false, err
	}
	response, err := client.Do(request)
	if err != nil {
		return false, nil
	}
	defer response.Body.Close()
	var health struct{ Name, Version, Repo, Status string }
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&health) != nil || health.Name != "stef" || health.Status != "ok" || health.Version != "0.3.0" {
		return false, fmt.Errorf("endpoint is not a compatible stef companion")
	}
	if health.Repo != root {
		return false, fmt.Errorf("stef at %s watches %s; choose a different --daemon port for %s", endpoint, health.Repo, root)
	}
	return true, nil
}

func ensureCompanion(ctx context.Context, endpoint, root string, headless, noAutostart bool, data string, ephemeral bool) (func(), error) {
	return ensureCompanionMode(ctx, endpoint, root, headless, noAutostart, data, ephemeral, false, 0)
}
func ensureCompanionMode(ctx context.Context, endpoint, root string, headless, noAutostart bool, data string, ephemeral, resident bool, lifetime time.Duration) (func(), error) {
	if err := mcp.ValidateEndpoint(endpoint); err != nil {
		return nil, err
	}
	if ready, err := probeCompanion(ctx, endpoint, root); err != nil {
		return nil, err
	} else if ready {
		return func() {}, nil
	}
	if noAutostart {
		return nil, fmt.Errorf("start stef for --repo before connecting with --no-autostart")
	}
	parsed, _ := url.Parse(endpoint)
	port := parsed.Port()
	if port == "" {
		port = "80"
	}
	host := parsed.Hostname()
	if host == "localhost" {
		host = "127.0.0.1"
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{"--repo", root, "--host", host, "--port", port}
	if lifetime > 0 {
		args = append(args, "--quit-after", lifetime.String())
	}
	if headless {
		args = append(args, "--headless", "--view", "ide")
	} else {
		args = append(args, "--view", "native")
	}
	if ephemeral {
		args = append(args, "--no-persist")
	}
	if data != "" {
		args = append(args, "--data-dir", data)
	}
	child := exec.Command(executable, args...)
	child.Dir = root
	child.Stdout = os.Stderr // The parent stdout is exclusively MCP JSON-RPC.
	child.Stderr = os.Stderr
	if resident {
		confined, err := os.OpenRoot(root)
		if err != nil {
			return nil, err
		}
		defer confined.Close()
		if err := confined.MkdirAll(".stef", 0700); err != nil {
			return nil, err
		}
		logFile, err := confined.OpenFile(".stef/companion.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		defer logFile.Close()
		child.Stdout = logFile
		child.Stderr = logFile
		child.SysProcAttr = residentProcessAttrs()
	}
	if err := child.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	stop := func() {
		if err := child.Process.Signal(os.Interrupt); err != nil {
			// Windows cannot send Interrupt; do not race the MCP client's deadline.
			_ = child.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			<-done
		}
	}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			return nil, fmt.Errorf("companion exited during startup: %v", err)
		case <-ctx.Done():
			stop()
			return nil, ctx.Err()
		case <-deadline.C:
			stop()
			return nil, fmt.Errorf("companion did not become ready")
		case <-tick.C:
			if ready, err := probeCompanion(ctx, endpoint, root); err != nil {
				stop()
				return nil, err
			} else if ready {
				return stop, nil
			}
		}
	}
}
