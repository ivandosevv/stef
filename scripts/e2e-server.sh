#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$project_root"
mkdir -p .test-workspace
test_root="$(mktemp -d "$project_root/.test-workspace/e2e.XXXXXX")"
git init -q "$test_root"
cp examples/sandbox/main.go "$test_root/main.go"
./bin/stef --headless --view ide --repo "$test_root" --port 4821 --ui-dir "$project_root/ui/dist" --no-persist &
daemon_pid=$!
cleanup() { kill "$daemon_pid" 2>/dev/null || true; wait "$daemon_pid" 2>/dev/null || true; rm -rf "$test_root"; }
trap cleanup EXIT INT TERM
wait "$daemon_pid"
