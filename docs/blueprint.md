Here is the comprehensive, production-grade architectural and implementation blueprint for your open-source repository on GitHub (`ivandosevv/agentlens`).

This plan is structured so you can feed it directly to a coding agent (like Claude Code or Cursor) to bootstrap the entire project file-by-file, starting with **Phase 1: The Core Local Daemon & Interactive HUD**.

---

# Repository Blueprint: `ivandosevv/agentlens`

**Tagline:** Real-time visual HUD and AST trajectory tracker for AI coding agents. Watch agents build your architecture live, inspect their "why" trail, and replay agent sessions in 6 seconds.

---

## 1. High-Level System Architecture

```
[Claude Code / Cursor / OpenCode] 
       │ (MCP stdio / Local Hooks)
       ▼
[Go Local Daemon (`agentlensd`)] ──(WebSocket / JSON)──► [Svelte / Vite Companion HUD]
       ▲                                                          (Runs in Browser / Tauri)
       │ (File System / Git Watcher via fsnotify)
[Local Repo / .git worktree]

```

* **Zero Cloud Dependencies:** Everything runs locally on the developer's machine.
* **Zero Token Bloat:** Piggybacks on the agent's active execution context via a local MCP server (`stdio`).
* **Zero Post-Work Tax:** Renders animations *while* the agent is generating code, allowing the developer to absorb the mental model in real time without breaking flow.

---

## 2. Monorepo Directory Structure

Create your repository with the following clean, modular structure:

```text
agentlens/
├── README.md
├── LICENSE (MIT)
├── daemon/                  # Go backend (MCP server + WebSocket + File watcher)
│   ├── go.mod
│   ├── go.sum
│   ├── main.go              # Entry point & CLI flags
│   ├── mcp/                 # Model Context Protocol stdio bridge
│   │   └── server.go
│   ├── watcher/             # fsnotify & Git worktree parser
│   │   └── watcher.go
│   └── hub/                 # WebSocket hub broadcasting state to UI
│       └── hub.go
└── ui/                      # Frontend companion HUD (React/Svelte + Tailwind + Canvas)
    ├── package.json
    ├── vite.config.ts
    ├── index.html
    └── src/
        ├── App.svelte       # Main Fleet / HUD View
        ├── components/
        │   ├── GraphCanvas.svelte # Animated node graph (Nodes, Edges, 5-word badges)
        │   ├── DVRReplay.svelte   # 6-second time-lapse replay scrubber
        │   └── CollisionRadar.svelte # Multi-agent conflict warning
        └── themes/
            ├── hacker.css   # Phosphor green CRT aesthetic
            ├── blueprint.css# CAD engineering schematic
            └── cozy.css     # Playful pixel block aesthetic

```

---

## 3. Step-by-Step Implementation Guide for Your Coding Agent

Feed this exact section to your coding agent to build **Phase 1 (The Go Daemon)**.

### Step 3.1: Initialize the Go Backend (`daemon/`)

Instruct your coding agent to run the following in your terminal:

```bash
mkdir -p agentlens/daemon agentlens/ui
cd agentlens/daemon
go mod init github.com/ivandosevv/agentlens/daemon
go get github.com/gorilla/websocket
go get github.com/fsnotify/fsnotify

```

#### File: `daemon/main.go`

Create the entry point that spins up the local WebSocket server and prepares the MCP bridge:

```go
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/ivandosevv/agentlens/daemon/hub"
	"github.com/ivandosevv/agentlens/daemon/mcp"
	"github.com/ivandosevv/agentlens/daemon/watcher"
)

func main() {
	port := flag.String("port", "4820", "Port for AgentLens companion HUD WebSocket")
	flag.Parse()

	// Initialize WebSocket Hub
	wsHub := hub.NewHub()
	go wsHub.Run()

	// Setup HTTP route for UI companion connection
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWs(wsHub, w, r)
	})

	// Start File / Git Worktree Watcher
	go watcher.StartWatcher(wsHub)

	// Start MCP Stdio Listener for agent telemetry
	go mcp.StartMCPServer(wsHub)

	log.Printf("🚀 AgentLens local daemon running on ws://localhost:%s/ws", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

```

#### File: `daemon/hub/hub.go`

Handles real-time broadcasting of agent node mutations to the companion UI:

```go
package hub

import (
	"encoding/json"
	"log"
	"net/http"
	"github.com/gorilla/websocket"
)

type AgentEvent struct {
	AgentID       string `json:"agent_id"`
	NodeName      string `json:"node_name"`
	Action        string `json:"action"` // "scouted", "created", "modified", "tested"
	Summary       string `json:"summary"` // Strictly 5 words max
	Why           string `json:"why"`     // 1 sentence reasoning
	Status        string `json:"status"`  // "running", "collision_warning", "ready"
}

type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		clients:    make(map[*websocket.Conn]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.Close()
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Printf("Websocket error: %v", err)
					client.Close()
					delete(h.clients, client)
				}
			}
		}
	}
}

func (h *Hub) BroadcastEvent(event AgentEvent) {
	data, err := json.Marshal(event)
	if err == nil {
		h.broadcast <- data
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	hub.register <- conn
	// Keep connection alive / read loop
	go func() {
		defer func() { hub.unregister <- conn }()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

```

---

## 4. Phase 2: The Interactive UI Companion (`ui/`)

For the UI, instruct your coding agent to scaffold a Vite + Svelte (or React) app styled with Tailwind CSS, featuring smooth SVG/Canvas graph rendering.

### Key UI Requirements for the Agent:

1. **The Live Canvas (`GraphCanvas.svelte`):** Renders glowing nodes as the WebSocket streams events from `daemon`. Each node displays a title and a crisp **5-word summary badge** beneath it (e.g., *"Injected sliding window rate limiter"*).
2. **The "Why" Popover:** Clicking any node opens a clean floating card displaying:
* **Agent's Decision Trail:** The exact 1-sentence "Why I went this path" rationale captured from the agent's context.
* **AST Code Diff:** A clean, minimal code diff for that specific function/struct.


3. **The 6-Second DVR Replay Bar (`DVRReplay.svelte`):** A playback scrubber at the bottom of the window that replays the exact chronological sequence of node connections at 10x speed so reviewers can understand the PR in seconds.
4. **Theme Switcher CSS (`themes/`):** Instant toggle between **Hacker (CRT Green)**, **Blueprint (CAD Grid)**, and **Cozy (Pastel Blocks)**.

---

## 5. How to Prompt Your Coding Agent Next

When you initialize your repository on GitHub (`ivandosevv/agentlens`), you can copy-paste this prompt to your coding agent to build the initial codebase:

> *"You are building `agentlens`, an open-source developer tool created by ivandosevv. Build the Go local daemon in `daemon/` with an MCP stdio bridge, a file watcher, and a WebSocket hub broadcasting live agent mutations. Then, build the Svelte/Tailwind companion UI in `ui/` featuring an animated node graph, 5-word summary badges, a 6-second DVR playback scrubber, and theme toggles for Hacker, Blueprint, and Cozy modes. Follow the monorepo architecture plan precisely."*

Would you like me to expand on the exact Model Context Protocol (MCP) tool schema spec that hooks Claude Code into this daemon next?