"""Native user-scope registration tests in isolated Claude profiles. No model calls."""
import importlib.util
import json
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import unittest
import urllib.request


ROOT = Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location("connect_claude", ROOT / "scripts/connect-claude.py")
connector = importlib.util.module_from_spec(spec)
spec.loader.exec_module(connector)


@unittest.skipUnless(shutil.which("claude"), "Claude Code CLI is required")
class PersonalConnectionTests(unittest.TestCase):
    def setUp(self):
        (ROOT / ".test-workspace").mkdir(exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(prefix="claude-connect.", dir=ROOT / ".test-workspace")
        self.workspace = Path(self.temporary.name)
        self.repo = self.workspace / "project with 'quotes'"
        self.repo.mkdir()
        self.config = self.workspace / "profile"
        self.config.mkdir()
        self.profile = self.config / ".claude.json"
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        self.endpoint = f"http://127.0.0.1:{port}"
        self.engine = subprocess.Popen([str(ROOT / "bin/stef"), "--headless", "--repo",
                                        str(self.repo), "--port", str(port), "--quit-after", "2m"],
                                       cwd=ROOT, stdin=subprocess.DEVNULL,
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        deadline = time.monotonic() + 10
        while True:
            try:
                with urllib.request.urlopen(self.endpoint + "/healthz", timeout=1):
                    break
            except OSError:
                if self.engine.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError("Test engine failed to start")
                time.sleep(0.1)

    def tearDown(self):
        self.engine.terminate()
        self.engine.wait(timeout=10)
        self.temporary.cleanup()

    def connect(self):
        return connector.connect(self.repo, self.config, self.endpoint)

    def test_native_registration_preserves_other_settings_and_binds_project(self):
        other = {"command": "/example/server", "args": ["--custom"]}
        self.profile.write_text(json.dumps({"mcpServers": {"other": other}, "theme": "light"}))
        result = self.connect()
        profile = json.loads(self.profile.read_text())
        self.assertEqual(profile["mcpServers"]["other"], other)
        self.assertEqual(profile["theme"], "light")
        server = profile["mcpServers"]["stef"]
        self.assertEqual(server["command"], str(ROOT / "bin/stef"))
        self.assertEqual(server["args"], ["--mcp", "--repo", str(self.repo), "--daemon", self.endpoint])
        skill = self.config / "skills/stef/SKILL.md"
        self.assertEqual(result["repo"], str(self.repo))
        self.assertTrue(skill.is_file())
        self.assertTrue(skill.resolve().is_relative_to(self.repo))
        self.assertIn(str(self.repo), skill.read_text())
        self.assertNotIn("{{", skill.read_text())

    def test_rerun_preserves_profile_exactly(self):
        self.connect()
        before = self.profile.read_bytes()
        self.connect()
        self.assertEqual(self.profile.read_bytes(), before)

    def test_conflicting_user_server_stops_before_project_setup(self):
        before = json.dumps({"mcpServers": {"stef": {"command": "/another/server", "args": []}}})
        self.profile.write_text(before)
        with self.assertRaisesRegex(ValueError, "different target"):
            self.connect()
        self.assertEqual(self.profile.read_text(), before)
        self.assertFalse((self.repo / ".mcp.json").exists())

    def test_personal_skill_is_not_overwritten(self):
        skill = self.config / "skills/stef"
        skill.mkdir(parents=True)
        (skill / "SKILL.md").write_text("Custom skill")
        with self.assertRaisesRegex(ValueError, "personal stef skill"):
            self.connect()
        self.assertEqual((skill / "SKILL.md").read_text(), "Custom skill")
        self.assertFalse(self.profile.exists())

    def test_home_is_not_an_observer_root(self):
        with self.assertRaisesRegex(ValueError, "home directory"):
            connector.connect(Path.home(), self.config, self.endpoint)
        self.assertFalse(self.profile.exists())

    def test_runtime_link_escape_stops_before_registration(self):
        outside = self.workspace / "outside"
        outside.mkdir()
        (self.repo / ".stef/claude-user").symlink_to(outside, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, "escapes"):
            self.connect()
        self.assertFalse(self.profile.exists())
        self.assertEqual(list(outside.iterdir()), [])

    def test_invalid_mcp_mapping_is_preserved(self):
        before = '{"mcpServers": []}'
        self.profile.write_text(before)
        with self.assertRaisesRegex(ValueError, "invalid MCP mapping"):
            self.connect()
        self.assertEqual(self.profile.read_text(), before)
        self.assertFalse((self.repo / ".mcp.json").exists())

    def test_skill_file_escape_stops_before_registration(self):
        runtime = self.repo / ".stef/claude-user/stef"
        runtime.mkdir(parents=True)
        outside = self.workspace / "outside.md"
        outside.write_text("Keep this source")
        (runtime / "SKILL.md").symlink_to(outside)
        with self.assertRaisesRegex(ValueError, "escapes"):
            self.connect()
        self.assertFalse(self.profile.exists())
        self.assertEqual(outside.read_text(), "Keep this source")


if __name__ == "__main__":
    unittest.main()
