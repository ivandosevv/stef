"""Call the existing local stef MCP bridge; useful for live integration checks."""
import argparse
import json
from pathlib import Path
import queue
import subprocess
import sys
import threading
import time

parser = argparse.ArgumentParser()
parser.add_argument("--repo", required=True)
parser.add_argument("--endpoint", required=True)
parser.add_argument("--tool", required=True, choices=["report_event", "report_changes", "get_state", "set_session", "connection"])
options = parser.parse_args()
root = Path(__file__).resolve().parent.parent
arguments = json.load(sys.stdin)
child = subprocess.Popen([str(root / "bin/stef"), "--mcp", "--no-autostart",
                          "--repo", options.repo, "--daemon", options.endpoint],
                         stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                         stderr=sys.stderr, text=True, bufsize=1)
responses = queue.Queue()
def receive():
    for line in child.stdout:
        try:
            responses.put(json.loads(line))
        except json.JSONDecodeError:
            responses.put({"error": "Non-protocol MCP stdout"})
threading.Thread(target=receive, daemon=True).start()
def send(message):
    child.stdin.write(json.dumps(message) + "\n")
    child.stdin.flush()
def call(number, method, params):
    send({"jsonrpc": "2.0", "id": number, "method": method, "params": params})
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        response = responses.get(timeout=max(0.01, deadline - time.monotonic()))
        if "error" in response:
            raise RuntimeError(response["error"])
        if response.get("id") == number:
            return response["result"]
    raise RuntimeError("MCP request timed out")
try:
    initialized = call(1, "initialize", {"protocolVersion": "2025-03-26", "capabilities": {},
                           "clientInfo": {"name": "stef-live-check", "version": "1"}})
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})
    if options.tool == "connection":
        catalog = call(2, "tools/list", {})
        result = {"structuredContent": {"connected": True, "server": initialized.get("serverInfo"),
                                        "instructions": initialized.get("instructions", ""),
                                        "tools": [tool["name"] for tool in catalog.get("tools", [])]}}
    else:
        result = call(2, "tools/call", {"name": options.tool, "arguments": arguments})
    if result.get("isError"):
        raise RuntimeError(result)
    json.dump(result.get("structuredContent", result), sys.stdout)
    print()
finally:
    child.stdin.close()
    try:
        child.wait(timeout=8)
    except subprocess.TimeoutExpired:
        child.terminate()
        child.wait(timeout=5)
