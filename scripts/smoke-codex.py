"""Opt-in real Codex task, isolated from user/global MCP settings and other repos.

Uses existing Codex authentication and makes real model requests. Hook trust is
bypassed only for this fixture's generated/vetted passive hooks, never globally.
"""
import json
import argparse
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import urllib.request

root = Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser()
parser.add_argument("--startup-only", action="store_true", help="Verify actual Codex launches stef, then stop before task validation")
options = parser.parse_args()
log_prefix = "codex-startup" if options.startup_only else "codex-smoke"
(root / ".test-workspace").mkdir(exist_ok=True)
(root / "artifacts").mkdir(exist_ok=True)
codex = shutil.which("codex")
if not codex:
    raise SystemExit("Codex CLI is required for this opt-in test")
with tempfile.TemporaryDirectory(prefix="codex.", dir=root / ".test-workspace") as fixture:
    workspace = Path(fixture)
    subprocess.run(["git", "init", "-q", fixture], check=True)
    (workspace / "calculator.py").write_text("def add(a, b):\n    return a - b\n")
    (workspace / "test_calculator.py").write_text("import unittest\nfrom calculator import add\nclass AdditionTest(unittest.TestCase):\n    def test_addition(self):\n        self.assertEqual(add(4, 3), 7)\n")
    (workspace / "AGENTS.md").write_text("Work only in this isolated fixture. Do not read outside it, access SAP or Hyperspace, connect other MCPs, or spawn agents. Fix calculator.py with apply_patch and run python3 -m unittest test_calculator.py.\n")
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}"
    subprocess.run([str(root / "bin/stef"), "--setup-codex", "--repo", fixture, "--daemon", endpoint], check=True)
    prompt = ("Fix the addition bug in calculator.py. Read the files, change only calculator.py using apply_patch, and run python3 -m unittest test_calculator.py. "
              "Use stef report_event twice: before the fix and after successful validation, agent_id codex-demo, node_name calculator.py, summary at most five words and why one short sentence, status ready after validation. "
              "Consider any developer note in the reply. Do not use other MCP servers, spawn agents, or read outside this fixture. Keep the final answer brief.")
    command = [codex, "exec", "--ignore-user-config", "--ignore-rules", "--ephemeral",
               "--dangerously-bypass-hook-trust", "--sandbox", "workspace-write", "--json",
               "-c", "approval_policy='never'", "-c", f'projects={{{json.dumps(fixture)}={{trust_level="trusted"}}}}',
               "-C", fixture, prompt]
    child = subprocess.Popen(command, cwd=fixture, stdin=subprocess.DEVNULL,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, bufsize=1)
    stdout, stderr = [], []
    def capture(stream, output):
        for line in stream:
            output.append(line)
    readers = [threading.Thread(target=capture, args=(child.stdout, stdout), daemon=True),
               threading.Thread(target=capture, args=(child.stderr, stderr), daemon=True)]
    for reader in readers:
        reader.start()
    latest = None
    note_sent = False
    deadline = time.monotonic() + 180
    try:
        while time.monotonic() < deadline:
            try:
                with urllib.request.urlopen(endpoint + "/api/events", timeout=1) as response:
                    latest = json.load(response)
                if not note_sent:
                    request = urllib.request.Request(endpoint + "/api/feedback",
                        data=json.dumps({"kind": "suggestion", "text": "Please keep the fix limited to calculator.py.", "target_agent": "codex-demo"}).encode(),
                        headers={"Content-Type": "application/json"})
                    with urllib.request.urlopen(request, timeout=1) as response:
                        assert response.status == 201
                    note_sent = True
                if options.startup_only:
                    child.terminate()
                    child.wait(timeout=15)
                    summary = {"result": "startup-passed", "client": subprocess.check_output([codex, "--version"], text=True).strip(),
                               "companion_autostart": True, "task_validation": "not attempted in startup-only mode", "global_config_changed": False}
                    (root / "artifacts/codex-startup-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
                    print(json.dumps(summary, indent=2))
                    break
            except OSError:
                pass
            if child.poll() is not None:
                break
            time.sleep(0.1)
        else:
            raise RuntimeError("Codex task exceeded 180 seconds")
        for reader in readers:
            reader.join(timeout=3)
        if options.startup_only:
            assert latest, "Codex did not launch stef; inspect the startup logs"
            # Child cleanup must close the MCP-owned observer.
            deadline = time.monotonic() + 10
            while time.monotonic() < deadline:
                try:
                    urllib.request.urlopen(endpoint + "/healthz", timeout=1)
                except OSError:
                    break
                time.sleep(0.1)
            else:
                raise AssertionError("MCP-owned engine outlived Codex")
            raise SystemExit(0)
        if child.returncode != 0:
            raise RuntimeError("Codex did not complete; see artifacts/codex-smoke.stderr.log")
        assert latest, "Codex did not launch stef"
        events = latest["events"]
        phases = {event.get("phase") for event in events if event.get("category") == "activity"}
        assert {"reading", "editing", "testing"} <= phases, f"Missing automatic activity: {phases}"
        assert any(event["source"] == "watcher" and event["node_name"] == "calculator.py" for event in events), "No observed code diff"
        assert any(event["source"] == "agent" and event["agent_id"] == "codex-demo" for event in events), "No actual MCP milestone"
        subprocess.run(["python3", "-m", "unittest", "test_calculator.py"], cwd=fixture, check=True, capture_output=True)
        assert any('"feedback"' in line and "Please keep the fix" in line for line in stdout), "Human note absent from Codex MCP reply"
        version = subprocess.check_output([codex, "--version"], text=True).strip()
        summary = {"result": "passed", "client": version, "phases": sorted(phase for phase in phases if phase), "events": len(events),
                   "file_diff": True, "mcp_milestone": True, "developer_note": True, "global_config_changed": False}
        (root / "artifacts/codex-smoke-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
        print(json.dumps(summary, indent=2))
    finally:
        if child.poll() is None:
            child.terminate()
            child.wait(timeout=15)
        (root / f"artifacts/{log_prefix}.jsonl").write_text("".join(stdout))
        (root / f"artifacts/{log_prefix}.stderr.log").write_text("".join(stderr))
        if latest:
            (root / f"artifacts/{log_prefix}-events.json").write_text(json.dumps(latest, indent=2) + "\n")
