"""Exercise actual stdio MCP -> native companion -> human note -> owned shutdown."""
import json
from pathlib import Path
import queue
import shutil
import socket
import subprocess
import tempfile
import threading
import time
import urllib.request

root = Path(__file__).resolve().parent.parent
(root / ".test-workspace").mkdir(exist_ok=True)
(root / "artifacts").mkdir(exist_ok=True)
with tempfile.TemporaryDirectory(prefix="native.", dir=root / ".test-workspace") as fixture:
    shutil.copyfile(root / "examples/sandbox/main.go", Path(fixture) / "main.go")
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    endpoint = f"http://127.0.0.1:{port}"
    child = subprocess.Popen([str(root / "bin/stef"), "--mcp", "--view", "native", "--repo", fixture,
                              "--daemon", endpoint, "--no-persist"],
                             cwd=root, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, text=True, bufsize=1)
    replies = queue.Queue()
    logs = []
    ready = threading.Event()
    def read_stdout():
        for line in child.stdout:
            try:
                replies.put(json.loads(line))
            except json.JSONDecodeError:
                replies.put({"error": f"Non-protocol stdout: {line}"})
    def read_stderr():
        for line in child.stderr:
            logs.append(line)
            if "stef native renderer ready" in line:
                ready.set()
    out_thread = threading.Thread(target=read_stdout, daemon=True)
    log_thread = threading.Thread(target=read_stderr, daemon=True)
    out_thread.start()
    log_thread.start()
    def send(message):
        child.stdin.write(json.dumps(message) + "\n")
        child.stdin.flush()
    def rpc(number, method, params):
        send({"jsonrpc": "2.0", "id": number, "method": method, "params": params})
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            response = replies.get(timeout=max(0.01, deadline - time.monotonic()))
            if "error" in response:
                raise RuntimeError(response["error"])
            if response.get("id") == number:
                return response["result"]
        raise RuntimeError("MCP response timed out")
    try:
        result = rpc(1, "initialize", {"protocolVersion": "2025-03-26", "capabilities": {},
                                      "clientInfo": {"name": "native-smoke", "version": "1"}})
        assert result["serverInfo"]["name"] == "stef"
        send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        assert ready.wait(15), "Native Svelte renderer did not mount"
        selected = rpc(2, "tools/call", {"name": "set_session", "arguments": {
            "session_id": "native-smoke-chat", "agent_id": "native-smoke"}})
        origin = selected["structuredContent"]["session_id"]
        note = {"kind": "question", "text": "Does this cover retries?",
                "target_agent": "native-smoke", "session_id": origin}
        request = urllib.request.Request(endpoint + "/api/feedback", data=json.dumps(note).encode(),
                                         headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(request, timeout=3) as response:
            assert response.status == 201
        result = rpc(3, "tools/call", {"name": "report_event", "arguments": {
            "session_id": origin, "agent_id": "native-smoke", "node_name": "main.go", "action": "tested",
            "summary": "Verified the native companion", "why": "Human notes arrive through a real MCP check-in.", "status": "ready"}})
        assert not result.get("isError"), result
        assert result["structuredContent"]["feedback"][0]["text"] == note["text"]
        child.stdin.close()
        assert child.wait(timeout=10) == 0
        log_thread.join(timeout=2)
        try:
            urllib.request.urlopen(endpoint + "/healthz", timeout=1)
        except OSError:
            pass
        else:
            raise AssertionError("Owned companion still running after MCP EOF")
        logs.append("PASS: native renderer, clean stdio, MCP feedback, owned shutdown\n")
        print(logs[-1].strip())
    finally:
        if child.poll() is None:
            child.terminate()
            child.wait(timeout=10)
        (root / "artifacts/native-smoke.log").write_text("".join(logs))
