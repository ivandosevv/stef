"""Opt-in Claude home-directory discovery with no explicit MCP config flags.

Uses a fresh user profile containing only stef, plus existing auth in process
memory. Makes a small real model request; no other user MCP servers are loaded.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request


ROOT = Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location("connect_claude", ROOT / "scripts/connect-claude.py")
connector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(connector)


def stop(child):
    if child.poll() is None:
        os.killpg(child.pid, signal.SIGTERM)
        try:
            child.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(child.pid, signal.SIGKILL)
            child.wait(timeout=5)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--auth-settings", type=Path, help="Read only authentication/provider env fields")
    options = parser.parse_args()
    (ROOT / ".test-workspace").mkdir(exist_ok=True)
    artifacts = ROOT / "artifacts"
    artifacts.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="claude-home.", dir=ROOT / ".test-workspace") as temporary:
        workspace = Path(temporary)
        repo, profile = workspace / "personal-project", workspace / "profile"
        repo.mkdir()
        profile.mkdir()
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        endpoint = f"http://127.0.0.1:{port}"
        engine = subprocess.Popen([str(ROOT / "bin/stef"), "--headless", "--repo",
                                   str(repo), "--port", str(port), "--quit-after", "3m"],
                                  cwd=ROOT, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                  stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            deadline = time.monotonic() + 10
            while True:
                try:
                    with urllib.request.urlopen(endpoint + "/healthz", timeout=1):
                        break
                except OSError:
                    if engine.poll() is not None or time.monotonic() > deadline:
                        raise RuntimeError("Home-discovery test engine did not start")
                    time.sleep(0.1)
            setup = connector.connect(repo, profile, endpoint)
            env = os.environ.copy()
            env["CLAUDE_CONFIG_DIR"] = str(profile)
            if options.auth_settings:
                auth = json.loads(options.auth_settings.expanduser().read_text()).get("env", {})
                allowed = {"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL",
                           "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_MODEL",
                           "ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL",
                           "ANTHROPIC_DEFAULT_OPUS_MODEL"}
                env.update({k: v for k, v in auth.items() if k in allowed and isinstance(v, str)})
            prompt = ("Load the stef skill and check its connection with get_state once, agent_id claude-home-smoke. "
                      "Do not edit, run commands, change settings, select sessions, or read other files. "
                      "Name the bound project and explain stef in one sentence.")
            command = ["claude", "-p", prompt, "--output-format", "stream-json", "--verbose",
                       "--no-session-persistence", "--permission-mode", "dontAsk",
                       "--tools", "Skill", "--allowedTools", "Skill(stef)",
                       "mcp__stef__get_state", "--effort", "low", "--max-budget-usd", "0.75"]
            stdout_path = artifacts / "claude-home-discovery.jsonl"
            with stdout_path.open("w") as stdout, (artifacts / "claude-home-discovery.stderr.log").open("w") as stderr:
                child = subprocess.Popen(command, cwd=Path.home(), env=env, stdin=subprocess.DEVNULL,
                                         stdout=stdout, stderr=stderr, start_new_session=True)
                try:
                    code = child.wait(timeout=90)
                finally:
                    stop(child)
            messages = [json.loads(line) for line in stdout_path.read_text().splitlines()]
            init = next((m for m in messages if m.get("subtype") == "init"), {})
            final = next((m for m in reversed(messages) if m.get("type") == "result"), {})
            assert code == 0 and not final.get("is_error"), "Claude home request did not succeed"
            assert init.get("mcp_servers") == [{"name": "stef", "status": "connected"}], "Automatic personal MCP discovery failed"
            assert "stef" in init.get("skills", []), "Personal stef skill was not discovered"
            calls = [b for m in messages for b in m.get("message", {}).get("content", [])
                     if isinstance(b, dict) and b.get("type") == "tool_use"]
            replies = {b["tool_use_id"]: b for m in messages for b in m.get("message", {}).get("content", [])
                       if isinstance(b, dict) and b.get("type") == "tool_result"}
            assert any(c["name"] == "Skill" and c["input"].get("skill") == "stef"
                       and not replies.get(c["id"], {"is_error": True}).get("is_error") for c in calls), "Claude did not load /stef"
            checks = [c for c in calls if c["name"] == "mcp__stef__get_state"]
            assert len(checks) == 1, "Expected one native MCP check-in"
            reply = replies[checks[0]["id"]]
            assert not reply.get("is_error") and str(repo) in json.dumps(reply), "MCP observed the wrong project"
            summary = {"result": "passed", "launch_directory": str(Path.home()), "observed_project": str(repo),
                       "scope": "user", "explicit_mcp_flags": False, "skill_discovered": True,
                       "skill_loaded": True, "mcp_connected": True, "mcp_checkins": 1,
                       "home_observed": False, "setup_skill_path": setup["skill_path"]}
            (artifacts / "claude-home-discovery-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
            print(json.dumps(summary))
        finally:
            stop(engine)


if __name__ == "__main__":
    main()
