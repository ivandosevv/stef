"""Opt-in real Claude Code tasks in a root-confined temporary fixture.

Uses existing authentication and model tokens. Loads only the generated stef MCP
and project hooks; leaves user settings and other projects alone.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import urllib.parse
import urllib.request
import uuid


ROOT = Path(__file__).resolve().parent.parent
AGENT = "claude-smoke"
NOTE = "Please keep the fix limited to calculator.mjs."


def request(endpoint, path, payload=None):
    data = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(endpoint + path, data=data,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=3) as response:
        return json.load(response)


def tool_calls(transcript):
    calls = []
    for line in transcript.splitlines():
        message = json.loads(line)
        for block in message.get("message", {}).get("content", []):
            if isinstance(block, dict) and block.get("type") == "tool_use":
                calls.append(block)
    return calls


def stop_process(child):
    if child.poll() is None:
        os.killpg(child.pid, signal.SIGTERM)
        try:
            child.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(child.pid, signal.SIGKILL)
            child.wait(timeout=5)


def tool_results(messages):
    results = {}
    for message in messages:
        for block in message.get("message", {}).get("content", []):
            if isinstance(block, dict) and block.get("type") == "tool_result":
                results[block["tool_use_id"]] = block
    return results


def is_test_command(command, test):
    try:
        tokens = shlex.split(command)
    except ValueError:
        return False
    if tokens and tokens[-1] == "2>&1":
        tokens.pop()
    return tokens == ["npm", "test", "--", f"--test-name-pattern={test}"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=["files", "milestones", "disabled"],
                        action="append", help="Repeat to test selected modes; default: all")
    parser.add_argument("--keep-view", action="store_true",
                        help="Keep the isolated view and fixture available for one hour")
    parser.add_argument("--startup-only", action="store_true",
                        help="Verify Claude loads stef and its hooks without claiming a completed task")
    parser.add_argument("--auth-settings", type=Path,
                        help="Read only authentication/provider env fields from this Claude settings file")
    options = parser.parse_args()
    modes = options.mode or ["files", "milestones", "disabled"]
    log_prefix = "claude-startup" if options.startup_only else "claude-smoke"
    claude = shutil.which("claude")
    if not claude:
        raise SystemExit("Claude Code CLI is required for this opt-in test")
    version = subprocess.check_output([claude, "--version"], text=True).strip()
    child_env = os.environ.copy()
    if options.auth_settings:
        auth = json.loads(options.auth_settings.expanduser().read_text()).get("env", {})
        allowed = {"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL",
                   "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_MODEL",
                   "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL",
                   "ANTHROPIC_DEFAULT_OPUS_MODEL"}
        child_env.update({key: value for key, value in auth.items()
                          if key in allowed and isinstance(value, str)})
    (ROOT / ".test-workspace").mkdir(exist_ok=True)
    artifacts = ROOT / "artifacts"
    artifacts.mkdir(exist_ok=True)
    workspace = Path(tempfile.mkdtemp(prefix="claude.", dir=ROOT / ".test-workspace"))
    child_env["CLAUDE_CONFIG_DIR"] = str(workspace / ".stef/claude-test-profile")
    subprocess.run(["git", "init", "-q", str(workspace)], check=True)
    (workspace / "calculator.mjs").write_text(
        "export const add = (a, b) => a - b;\n"
        "export const subtract = (a, b) => a + b;\n"
        "export const multiply = (a, b) => a + b;\n")
    (workspace / "calculator.test.mjs").write_text(
        "import { test } from 'node:test';\nimport assert from 'node:assert/strict';\n"
        "import { add, subtract, multiply } from './calculator.mjs';\n"
        "test('addition', () => assert.equal(add(4, 3), 7));\n"
        "test('subtraction', () => assert.equal(subtract(4, 3), 1));\n"
        "test('multiplication', () => assert.equal(multiply(4, 3), 12));\n")
    (workspace / "package.json").write_text(json.dumps({
        "private": True, "scripts": {"test": "node --test"}}, indent=2) + "\n")
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}"
    engine_log = (artifacts / f"{log_prefix}-engine.log").open("w")
    engine = subprocess.Popen([str(ROOT / "bin/stef"), "--repo", str(workspace),
                               "--port", str(port), "--headless", "--quit-after", "1h"],
                              cwd=workspace, stdin=subprocess.DEVNULL,
                              stdout=engine_log, stderr=engine_log, start_new_session=True)
    results = []
    seed_files = {name: (workspace / name).read_text()
                  for name in ["calculator.test.mjs", "package.json"]}
    try:
        deadline = time.monotonic() + 15
        while True:
            try:
                health = request(endpoint, "/healthz")
                assert health["repo"] == str(workspace)
                break
            except OSError:
                if engine.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError("Isolated stef engine did not start")
                time.sleep(0.1)
        setup = subprocess.check_output([str(ROOT / "bin/stef"), "--enable", "--client",
                                        "claude", "--view", "ide", "--repo",
                                        str(workspace), "--daemon", endpoint], text=True)
        assert json.loads(setup)["client_configured"]
        print(json.dumps({"view_url": endpoint, "workspace": str(workspace),
                          "client": version}), flush=True)
        tasks = {"files": ("add", "addition"),
                 "milestones": ("subtract", "subtraction"),
                 "disabled": ("multiply", "multiplication")}
        for mode in modes:
            request(endpoint, "/api/settings", {"update_mode": mode})
            session_uuid = str(uuid.uuid4())
            session = "claude-" + hashlib.sha256(session_uuid.encode()).hexdigest()[:32]
            if mode != "disabled":
                request(endpoint, "/api/feedback", {"session_id": session,
                    "target_agent": AGENT, "kind": "suggestion", "text": NOTE})
            function, test = tasks[mode]
            prompt = (f"Fix only the {function} bug in calculator.mjs. Read the relevant files, "
                      f"use Edit, and run npm test -- --test-name-pattern={test}. "
                      f"stef is connected. At task start use get_state with agent_id {AGENT} "
                      f"and session_id {session}, then follow its reporting instructions. "
                      "Share short reasons when the selected mode permits. When updates are enabled, "
                      "report the successful test as report_event with action tested and status ready. "
                      "Consider any developer note in the receipts. Stay inside this fixture; "
                      "do not use other servers, read other projects, or spawn agents. Keep the final brief.")
            if options.startup_only:
                prompt = "Reply ready. Do not edit files or call tools."
            command = [claude, "-p", prompt, "--output-format", "stream-json", "--verbose",
                       "--no-session-persistence", "--session-id", session_uuid,
                       "--setting-sources", "project,local",
                       "--permission-mode", "dontAsk", "--tools", "Read,Edit,Bash",
                       "--allowedTools", "Read", "Edit", "Bash(npm test*)",
                       "mcp__stef__get_state", "mcp__stef__set_session",
                       "mcp__stef__report_event", "mcp__stef__report_changes",
                       "--effort", "low", "--max-budget-usd", "1.50"]
            stdout_path = artifacts / f"{log_prefix}-{mode}.jsonl"
            stderr_path = artifacts / f"{log_prefix}-{mode}.stderr.log"
            with stdout_path.open("w") as stdout, stderr_path.open("w") as stderr:
                child = subprocess.Popen(command, cwd=workspace, stdin=subprocess.DEVNULL,
                                         stdout=stdout, stderr=stderr, env=child_env, start_new_session=True)
                try:
                    if options.startup_only:
                        deadline = time.monotonic() + 30
                        while child.poll() is None and time.monotonic() < deadline:
                            try:
                                started = [json.loads(line) for line in stdout_path.read_text().splitlines()]
                            except json.JSONDecodeError:
                                started = []
                            if any(message.get("subtype") == "init" for message in started):
                                break
                            time.sleep(0.1)
                        code = child.poll()
                    else:
                        code = child.wait(timeout=180)
                except subprocess.TimeoutExpired:
                    raise RuntimeError(f"Claude {mode} task exceeded 180 seconds")
                finally:
                    stop_process(child)
            transcript = stdout_path.read_text()
            messages = [json.loads(line) for line in transcript.splitlines()]
            connected = any(server.get("name") == "stef" and server.get("status") == "connected"
                            for message in messages for server in message.get("mcp_servers", []))
            assert connected, f"Claude did not load stef; see {stderr_path}"
            assert any("stef" in message.get("skills", []) for message in messages), "Project /stef was not discovered"
            if options.startup_only:
                hooks_loaded = any(message.get("subtype") == "hook_response" for message in messages)
                assert hooks_loaded, "Claude did not load project hooks"
                summary = {"client": version, "result": "startup-passed",
                           "mcp_connected": True, "project_hooks_loaded": True,
                           "task_validation": "not verified in startup-only mode",
                           "global_config_changed": False, "view_url": endpoint,
                           "workspace": str(workspace), "view_kept": options.keep_view}
                (artifacts / "claude-startup-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
                print(json.dumps(summary), flush=True)
                return
            final = next((message for message in reversed(messages) if message.get("type") == "result"), {})
            if code != 0 or final.get("is_error"):
                summary = {"client": version, "result": "task-blocked", "mode": mode,
                           "mcp_connected": connected, "task_completed": False,
                           "reason": final.get("result", f"Claude exited {code}"),
                           "view_url": endpoint, "workspace": str(workspace),
                           "completed_modes": results, "global_config_changed": False}
                (artifacts / "claude-smoke-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
                raise RuntimeError(f"Claude {mode} task blocked: {summary['reason']}")
            assert final.get("subtype") == "success", f"Claude {mode} did not finish: {final.get('subtype')}"
            calls = tool_calls(transcript)
            names = [call["name"] for call in calls]
            assert "Edit" in names and "Bash" in names, f"Claude did not edit and validate: {names}"
            replies = tool_results(messages)
            validation = [call for call in calls if call["name"] == "Bash"
                          and is_test_command(call.get("input", {}).get("command", ""), test)]
            assert validation, "Claude did not run the requested test command"
            assert any(not replies.get(call["id"], {"is_error": True}).get("is_error")
                       and "fail 0" in json.dumps(replies[call["id"]])
                       for call in validation), "Claude's actual test run did not pass"
            for name, seed in seed_files.items():
                assert (workspace / name).read_text() == seed, f"Claude changed test support file {name}"
            subprocess.run(["npm", "test", "--", f"--test-name-pattern={test}"],
                           cwd=workspace, check=True, capture_output=True)
            time.sleep(0.5)
            snapshot = request(endpoint, "/api/events?" + urllib.parse.urlencode({"session_id": session}))
            (artifacts / f"claude-smoke-{mode}-events.json").write_text(json.dumps(snapshot, indent=2) + "\n")
            events = snapshot.get("events", [])
            assert request(endpoint, "/api/events")["session_id"] == session, "Claude hook did not select its chat"
            if mode == "files":
                revisions = [e for e in events if e["source"] == "watcher"
                             and e["node_name"] == "calculator.mjs" and e.get("kind") == "file"]
                assert revisions, "Missing exact saved diff"
                for revision in revisions:
                    assert revision.get("diff"), "Saved diff missing"
                    assert any(e.get("change_id") == revision["id"] and e.get("why")
                               and e["source"] == "agent" for e in events), "Missing Claude revision explanation"
                for edit_index, name in enumerate(names):
                    if name != "Edit" or replies.get(calls[edit_index]["id"], {"is_error": True}).get("is_error"):
                        continue
                    next_edit_or_test = next((i for i in range(edit_index + 1, len(names))
                                              if names[i] in {"Edit", "Bash"}), len(names))
                    assert any(calls[i]["name"] == "mcp__stef__report_changes"
                               and not replies.get(calls[i]["id"], {"is_error": True}).get("is_error")
                               for i in range(edit_index + 1, next_edit_or_test)), "Reason arrived after the next edit or validation"
                phases = {e.get("phase") for e in events if e["source"] == "hook"}
                assert {"reading", "editing", "testing"} <= phases, f"Missing lifecycle phases: {phases}"
                state = request(endpoint, "/api/check-in", {"session_id": session})
                assert state["pending_count"] == 0, "Unexplained file saves remain"
            elif mode == "milestones":
                assert any(e["source"] == "agent" and e.get("why") for e in events), "Missing major milestone"
                assert any(e["source"] == "agent" and e["action"] == "tested"
                           and e["status"] == "ready" for e in events), "Missing validated milestone outcome"
                assert not any(e["source"] in {"watcher", "hook"} or e.get("change_id") for e in events), "Per-file recording in milestones"
                assert "mcp__stef__report_changes" not in names, "Unnecessary per-file report"
            else:
                assert not events, "Disabled recorded new events"
                assert names.count("mcp__stef__get_state") == 1, "Disabled repeated check-ins"
                assert not any(name in names for name in ["mcp__stef__report_event", "mcp__stef__report_changes"]), "Disabled sent reports"
            if mode != "disabled":
                assert any(NOTE in json.dumps(replies.get(call["id"], {}))
                           and not replies.get(call["id"], {"is_error": True}).get("is_error")
                           for call in calls if call["name"].startswith("mcp__stef__")), "Developer note absent from Claude MCP reply"
            result = {"mode": mode, "result": "passed", "session_id": session,
                      "events": len(events), "tool_calls": names,
                      "model_cost_usd": final.get("total_cost_usd")}
            results.append(result)
            print(json.dumps(result), flush=True)
        if options.keep_view:
            visible = next((result for result in results if result["mode"] == "files"), results[0])
            request(endpoint, "/api/settings", {"update_mode": visible["mode"]})
            request(endpoint, "/api/sessions/activate", {"session_id": visible["session_id"]})
            restored = request(endpoint, "/api/events")
            assert len(restored.get("events", [])) == visible["events"], "Chat restore changed its saved workflow"
        summary = {"client": version, "result": "passed", "modes": results,
                   "view_url": endpoint, "workspace": str(workspace),
                   "global_config_changed": False, "view_kept": options.keep_view}
        (artifacts / "claude-smoke-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    finally:
        engine_log.close()
        if not options.keep_view:
            stop_process(engine)
            shutil.rmtree(workspace)


if __name__ == "__main__":
    main()
