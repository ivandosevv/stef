#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd "$(dirname "$0")/.." && pwd)"
if [ "$#" -ge 1 ]; then repo_root="$1"; else repo_root="$project_root/examples/sandbox"; fi
if [ "$#" -ge 2 ]; then daemon_port="$2"; else daemon_port=4820; fi
mkdir -p "$project_root/bin"
cd "$project_root/daemon"
go build -o "$project_root/bin/stefd" .
"$project_root/bin/stefd" --repo "$repo_root" --port "$daemon_port" --ui-dir "$project_root/ui/dist" &
daemon_pid=$!
cleanup() { kill "$daemon_pid" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
cd "$project_root/ui"
STEF_DAEMON="http://127.0.0.1:$daemon_port" npm run dev
