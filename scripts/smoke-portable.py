"""Exercise a portable stef binary locally without an AI client or build tools."""
import argparse
import json
import os
from pathlib import Path
import platform
import queue
import socket
import subprocess
import tempfile
import threading
import time
import urllib.request

ROOT = Path(__file__).resolve().parent.parent


def request(endpoint, path):
    with urllib.request.urlopen(endpoint + path, timeout=2) as response:
        return json.load(response)


def stop(process):
    if process is None or process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=5)


def smoke(binary):
    binary = Path(binary).resolve(strict=True)
    (ROOT / ".test-workspace").mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(dir=ROOT / ".test-workspace", prefix="portable.") as directory:
        project = Path(directory)
        changed = project / "greeting.py"
        before, after = "message = 'old'" + chr(10), "message = 'new'" + chr(10)
        changed.write_bytes(before.encode("utf-8"))
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            port = reservation.getsockname()[1]
        endpoint = f"http://127.0.0.1:{port}"
        # The shipped runtime must work without Git, Go, Node, Bash, or make.
        environment = os.environ.copy()
        environment["PATH"] = ""
        daemon = bridge = None
        with (project / "runtime.log").open("w", encoding="utf-8") as log:
            try:
                daemon = subprocess.Popen([str(binary), "--repo", str(project), "--headless", "--port", str(port), "--quit-after", "30s"],
                                          cwd=project, env=environment, stdin=subprocess.DEVNULL, stdout=log, stderr=log)
                deadline = time.monotonic() + 10
                while True:
                    try:
                        health = request(endpoint, "/healthz")
                        break
                    except OSError:
                        if daemon.poll() is not None or time.monotonic() >= deadline:
                            raise RuntimeError("Portable daemon failed to start")
                        time.sleep(0.1)
                if health.get("name") != "stef" or Path(health.get("repo", "")) != project:
                    raise RuntimeError("Portable daemon attached to a different project")
                bridge = subprocess.Popen([str(binary), "--mcp", "--repo", str(project), "--daemon", endpoint, "--no-autostart", "--view", "ide"],
                                          cwd=project, env=environment, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=log, text=True, encoding="utf-8", bufsize=1)
                replies = queue.Queue()
                def receive():
                    try:
                        for line in bridge.stdout:
                            replies.put(json.loads(line))
                    except (OSError, ValueError) as error:
                        replies.put({"error": str(error)})
                threading.Thread(target=receive, daemon=True).start()
                def send(message):
                    bridge.stdin.write(json.dumps(message) + chr(10))
                    bridge.stdin.flush()
                def call(number, method, params):
                    send({"jsonrpc": "2.0", "id": number, "method": method, "params": params})
                    deadline = time.monotonic() + 10
                    while True:
                        reply = replies.get(timeout=max(0.01, deadline - time.monotonic()))
                        if "error" in reply:
                            raise RuntimeError(str(reply["error"]))
                        if reply.get("id") == number:
                            return reply["result"]
                def tool(number, name, arguments):
                    result = call(number, "tools/call", {"name": name, "arguments": arguments})
                    if result.get("isError"):
                        raise RuntimeError(str(result))
                    return result["structuredContent"]
                initialized = call(1, "initialize", {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "stef-portable-check", "version": "1"}})
                if initialized.get("serverInfo", {}).get("name") != "stef":
                    raise RuntimeError("Portable MCP bridge did not identify as stef")
                send({"jsonrpc": "2.0", "method": "notifications/initialized"})
                catalog = call(2, "tools/list", {})
                if {entry["name"] for entry in catalog["tools"]} != {"set_session", "set_workspace", "get_state", "report_event", "report_changes"}:
                    raise RuntimeError("Portable MCP tool catalog is incomplete")
                selected = tool(3, "set_session", {"session_id": "portable-smoke-chat", "agent_id": "portable-smoke-worker"})
                origin = selected["session_id"]
                if selected["shared"] or not selected["workspace_name"] or not selected["view_path"]:
                    raise RuntimeError("Portable worker did not start privately with a scoped view")
                changed.write_bytes(after.encode("utf-8"))
                state = tool(4, "get_state", {"session_id": origin, "agent_id": "portable-smoke-worker", "changed_paths": ["greeting.py"], "settle_ms": 1000})
                pending = state["pending_changes"]
                if state["pending_count"] != 1 or pending[0]["node_name"] != "greeting.py":
                    raise RuntimeError("Portable watcher did not capture the exact saved file")
                exact = request(endpoint, "/api/events?session_id=" + origin)["events"]
                observed = next(event for event in exact if event["id"] == pending[0]["change_id"])
                if observed.get("before") != before or observed.get("after") != after or "-message = 'old'" not in observed.get("diff", "") or "+message = 'new'" not in observed.get("diff", ""):
                    raise RuntimeError("Portable revision diff lost exact source content")
                explained = tool(5, "report_changes", {"session_id": origin, "agent_id": "portable-smoke-worker", "changes": [{"change_id": pending[0]["change_id"], "summary": "Update greeting", "why": "Show the new greeting while retaining the exact saved revision."}]})
                if not explained.get("accepted") or explained["pending_count"] != 0:
                    raise RuntimeError("Portable revision explanation did not resolve the saved change")
                bridge.stdin.close()
                bridge.wait(timeout=8)
                return {"os": platform.system(), "arch": platform.machine(), "binary": str(binary), "mcp": True, "private_workspace": True, "exact_saved_diff": True, "external_tools_required": False}
            finally:
                stop(bridge)
                stop(daemon)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, default=ROOT / "bin" / ("stef.exe" if os.name == "nt" else "stef"))
    parser.add_argument("--output", type=Path, help="Optional JSON verification receipt")
    options = parser.parse_args()
    result = smoke(options.binary)
    if options.output:
        options.output.parent.mkdir(parents=True, exist_ok=True)
        options.output.write_text(json.dumps(result, indent=2) + chr(10), encoding="utf-8")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
