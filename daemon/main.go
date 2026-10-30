package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ivandosevv/stef/daemon/companion"
	"github.com/ivandosevv/stef/daemon/hooks"
	"github.com/ivandosevv/stef/daemon/hub"
	"github.com/ivandosevv/stef/daemon/mcp"
	clientsetup "github.com/ivandosevv/stef/daemon/setup"
	"github.com/ivandosevv/stef/daemon/watcher"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	repo := flag.String("repo", ".", "Explicit repository root to watch")
	host := flag.String("host", "127.0.0.1", "Loopback interface to bind")
	port := flag.String("port", "4820", "HUD and WebSocket port")
	ui := flag.String("ui-dir", "", "Optional external UI directory (default: bundled UI)")
	data := flag.String("data-dir", "", "Event journal directory (default: <repo>/.stef)")
	ephemeral := flag.Bool("no-persist", false, "Keep history only in memory")
	bridge := flag.Bool("mcp", false, "Run stdio MCP and launch or reuse a companion for --repo")
	setupCodex := flag.Bool("setup-codex", false, "Configure project-local Codex MCP and passive lifecycle hooks for --repo")
	enable := flag.Bool("enable", false, "Enable stef for --client codex or claude and open a resident companion immediately")
	view := flag.String("view", "", "Presentation: native or ide; --enable saves the choice for this project")
	headless := flag.Bool("headless", filepath.Base(os.Args[0]) == "stefd", "Run the local engine without a native window")
	noAutostart := flag.Bool("no-autostart", false, "Require an already-running root-matched engine in MCP mode")
	quitAfter := flag.Duration("quit-after", 0, "Optional shutdown duration for demos and smoke tests")
	hook := flag.Bool("hook", false, "Forward a lifecycle hook from stdin; failures do not block the agent")
	client := flag.String("client", "generic", "Hook format: generic, claude, or codex")
	endpoint := flag.String("daemon", "http://127.0.0.1:4820", "Daemon origin for MCP bridge mode")
	flag.Parse()
	explicitEndpoint := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "daemon" {
			explicitEndpoint = true
		}
	})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *quitAfter > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, *quitAfter)
		defer stop()
	}
	if *hook {
		if err := hooks.Send(ctx, *endpoint, *client, os.Stdin); err != nil {
			log.Printf("stef hook skipped: %v", err)
		}
		return nil
	}
	ip := net.ParseIP(*host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--host must be a loopback IP")
	}
	number, err := strconv.Atoi(*port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("--port must be 1–65535")
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("--repo must be an existing directory")
	}
	presentation, err := projectPresentation(root, *view)
	if err != nil {
		return err
	}
	if presentation == "ide" {
		*headless = true
	}
	if *setupCodex || *enable {
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		selectedClient := *client
		if *setupCodex {
			selectedClient = "codex"
		}
		if selectedClient != "codex" && selectedClient != "claude" {
			return fmt.Errorf("--enable requires --client codex or claude")
		}
		if *enable && !explicitEndpoint {
			value, err := clientsetup.Endpoint(root, selectedClient)
			if err != nil {
				return err
			}
			*endpoint = value
		}
		configure := clientsetup.Codex
		if selectedClient == "claude" {
			configure = clientsetup.Claude
		}
		if err := configure(root, executable, *endpoint); err != nil {
			return err
		}
		if *enable {
			if _, err := ensureCompanionMode(ctx, *endpoint, root, *headless, false, *data, *ephemeral, true, *quitAfter); err != nil {
				return err
			}
			if *view != "" {
				if err := savePresentation(root, presentation); err != nil {
					return err
				}
			}
			next := "Restart the Codex extension once to load MCP; hook trust is reviewed with /hooks in the Codex CLI."
			if selectedClient == "claude" {
				next = "Use Claude Code's /mcp menu to approve/reconnect the project server; restart the session if needed."
			}
			result := map[string]any{"observing": true, "client": selectedClient, "repo": root, "endpoint": *endpoint, "view": presentation, "view_url": strings.TrimSuffix(*endpoint, "/") + "/?companion=1", "next_step": next}
			if presentation == "ide" {
				result["open_command"] = "workbench.action.browser.open"
				result["view_opened"] = false // The agent's host opens its built-in browser.
			}
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		fmt.Printf("stef configured in %s/.codex. Start Codex in this trusted project; review the passive hooks once with /hooks.\n", root)
		return nil
	}
	if *bridge {
		stop, err := ensureCompanion(ctx, *endpoint, root, *headless, *noAutostart, *data, *ephemeral)
		if err != nil {
			return err
		}
		defer stop()
		return mcp.Run(ctx, *endpoint)
	}
	if !*headless && !companion.Available() {
		return fmt.Errorf("native companion currently supports macOS; use --headless")
	}
	// Bind before scanning or opening the journal: a second daemon must not modify it.
	listener, err := net.Listen("tcp", net.JoinHostPort(*host, *port))
	if err != nil {
		return fmt.Errorf("bind daemon: %w", err)
	}
	defer listener.Close()
	journal := ""
	if !*ephemeral {
		directory := *data
		if directory == "" {
			directory = filepath.Join(root, ".stef")
		}
		if err := os.MkdirAll(directory, 0700); err != nil {
			return err
		}
		journal = filepath.Join(directory, "events.jsonl")
	}
	h, err := hub.New(root, journal)
	if err != nil {
		return err
	}
	defer h.Close()
	observer, err := watcher.New(root, h)
	if err != nil {
		return err
	}
	defer observer.Close()
	uiRoot := ""
	if *ui != "" {
		uiRoot, err = filepath.Abs(*ui)
		if err != nil {
			return err
		}
	}
	server := &http.Server{Handler: handler(h, uiRoot, *port), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	watchErrors := make(chan error, 1)
	go func() { watchErrors <- observer.Run(ctx) }()
	log.Printf("stef HUD: http://%s · watching %s", listener.Addr(), root)
	if !*headless {
		// AppKit's event loop belongs on the main thread. Engine failures terminate it.
		go func() {
			select {
			case err := <-serverErrors:
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Print(err)
				}
				cancel()
			case err := <-watchErrors:
				if err != nil {
					log.Print(err)
				}
				cancel()
			case <-ctx.Done():
			}
		}()
		windowErr := companion.Open(ctx, "http://"+listener.Addr().String(), "stef · "+filepath.Base(root), sessionExporter(root))
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := server.Shutdown(shutdown); err != nil {
			return err
		}
		return windowErr
	}
	select {
	case <-ctx.Done():
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case err := <-watchErrors:
		if err != nil {
			return err
		}
	}
	cancel()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	return server.Shutdown(shutdown)
}
