"""Run the stock Windows installer against local release fixtures and a real stef.exe."""
import argparse
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts/bootstrap.ps1"
POWERSHELL = shutil.which("powershell.exe") or shutil.which("pwsh")


@unittest.skipUnless(os.name == "nt" and POWERSHELL, "Run with Windows PowerShell on the Windows CI runner.")
class WindowsInstallerTest(unittest.TestCase):
    def setUp(self):
        binary = ROOT / "bin/stef.exe"
        if not binary.is_file():
            self.fail("Build the portable Windows binary before installer verification.")
        (ROOT / ".test-workspace").mkdir(exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=ROOT / ".test-workspace", prefix="install-ps1.")
        self.base = Path(self.temporary.name)
        self.project = self.base / "Current project λ & literal $()"
        self.project.mkdir()
        self.binary_bytes = binary.read_bytes()
        self.environment = os.environ.copy()
        self.map_file = self.base / "resources.json"
        self.environment["STEF_INSTALL_FIXTURES"] = str(self.map_file)
        # Replace only the external HTTP boundary. All validation, extraction,
        # installation and actual --enable behavior are the shipped script.
        content = SCRIPT.read_text()
        start = content.index("function Get-StefResource(")
        end = content.index("\nfunction Assert-StefAsset(", start)
        mock = """function Get-StefResource([string]$Url, [long]$Maximum) {
    $map = Get-Content -LiteralPath $env:STEF_INSTALL_FIXTURES -Raw | ConvertFrom-Json
    $property = $map.PSObject.Properties[$Url]
    if (-not $property) { throw 'Unexpected request outside the release fixtures.' }
    $data = [IO.File]::ReadAllBytes($property.Value)
    if ($data.Length -gt $Maximum) { throw 'Fixture exceeds download limit.' }
    return ,$data
}
"""
        self.harness = self.base / "bootstrap.ps1"
        self.harness.write_text(content[:start] + mock + content[end:])
        self.installed = self.project / ".stef/runtime/bin/stef.exe"
        self.addCleanup(self.temporary.cleanup)
        self.addCleanup(self.stop_owned_companion)
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            self.port = listener.getsockname()[1]
        self.endpoint = "http://127.0.0.1:" + str(self.port)
        self.fixtures()

    def fixtures(self, *, checksum=None, unsafe_path=None):
        architecture = self.environment.get("PROCESSOR_ARCHITEW6432") or self.environment.get("PROCESSOR_ARCHITECTURE")
        goarch = "arm64" if architecture == "ARM64" else "amd64"
        name = "stef-0.3.0-windows-" + goarch + ".zip"
        output = io.BytesIO()
        with zipfile.ZipFile(output, "w") as bundle:
            bundle.writestr("stef.exe", self.binary_bytes)
            bundle.writestr("LICENSE", (ROOT / "LICENSE").read_bytes())
            if unsafe_path:
                bundle.writestr(unsafe_path, b"must not escape")
        payload = output.getvalue()
        archive = self.base / "fixture.zip"
        archive.write_bytes(payload)
        hashes = self.base / "fixture.sha256"
        hashes.write_text((checksum or hashlib.sha256(payload).hexdigest()) + "  " + name + "\n")
        prefix = "https://github.com/ivandosevv/stef/releases/download/v0.3.0/"
        metadata = self.base / "release.json"
        metadata.write_text(json.dumps({"tag_name": "v0.3.0", "draft": False, "prerelease": False, "assets": [
            {"name": name, "browser_download_url": prefix + name},
            {"name": name + ".sha256", "browser_download_url": prefix + name + ".sha256"}]}))
        self.map_file.write_text(json.dumps({"https://api.github.com/repos/ivandosevv/stef/releases/latest": str(metadata),
                                             prefix + name: str(archive), prefix + name + ".sha256": str(hashes)}))

    def run_installer(self, *arguments, success=True):
        command = [POWERSHELL, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", str(self.harness),
                   "-Repo", str(self.project), "-Client", "generic", "-Daemon", self.endpoint, *arguments]
        result = subprocess.run(command, env=self.environment, capture_output=True, text=True, encoding="utf-8", timeout=60)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            return json.loads(result.stdout)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def stop_owned_companion(self):
        environment = self.environment.copy()
        environment["STEF_TEST_BINARY"] = str(self.installed)
        script = "Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq $env:STEF_TEST_BINARY } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }"
        subprocess.run([POWERSHELL, "-NoProfile", "-NonInteractive", "-Command", script], env=environment, capture_output=True, timeout=20)
        time.sleep(0.2)

    def test_shipped_script_parses_with_stock_windows_powershell(self):
        environment = self.environment.copy()
        environment["STEF_TEST_SCRIPT"] = str(SCRIPT)
        code = "$tokens=$null; $errors=$null; $null=[System.Management.Automation.Language.Parser]::ParseFile($env:STEF_TEST_SCRIPT,[ref]$tokens,[ref]$errors); if ($errors.Count) { $errors | ForEach-Object { [Console]::Error.WriteLine($_.Message) }; exit 1 }"
        result = subprocess.run([POWERSHELL, "-NoProfile", "-NonInteractive", "-Command", code], env=environment, capture_output=True, text=True, encoding="utf-8", timeout=20)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_release_install_enables_in_current_project_and_reinstalls_while_running(self):
        first = self.run_installer("-BuildOnly")
        self.assertEqual(Path(first["executable"]), self.installed)
        self.assertEqual(self.installed.read_bytes(), self.binary_bytes)
        self.assertFalse(first["enabled"])
        enabled = self.run_installer()
        self.assertFalse(enabled["client_configured"])
        self.assertFalse(enabled["mcp_connected"])
        self.assertFalse(enabled["view_opened"])
        self.assertEqual(Path(enabled["repo"]), self.project)
        with urllib.request.urlopen(self.endpoint + "/healthz", timeout=5) as response:
            self.assertEqual(Path(json.load(response)["repo"]), self.project)
        second = self.run_installer()
        self.assertEqual(second["endpoint"], self.endpoint)
        self.assertEqual(Path(second["executable"]), self.installed)
        self.assertEqual(self.installed.read_bytes(), self.binary_bytes)
        self.assertTrue((self.project / ".stef/clients/generic.json").is_file())

    def test_checksum_and_traversal_fail_without_replacing_the_previous_binary(self):
        self.run_installer("-BuildOnly")
        before = self.installed.read_bytes()
        self.fixtures(checksum="0" * 64)
        result = self.run_installer("-BuildOnly", success=False)
        self.assertIn("checksum", result.stderr.lower())
        self.assertEqual(self.installed.read_bytes(), before)
        self.fixtures(unsafe_path="../escaped.txt")
        result = self.run_installer("-BuildOnly", success=False)
        self.assertIn("unsafe", result.stderr.lower())
        self.assertEqual(self.installed.read_bytes(), before)
        self.assertFalse((self.base / "escaped.txt").exists())

    def test_python_release_install_keeps_unicode_from_the_actual_go_receipt(self):
        spec = importlib.util.spec_from_file_location("stef_windows_installer", ROOT / "install.py")
        installer = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(installer)
        resources = json.loads(self.map_file.read_text())
        def fetch(url, maximum):
            data = Path(resources[url]).read_bytes()
            self.assertLessEqual(len(data), maximum)
            return data
        arguments = argparse.Namespace(repo=str(self.project), client="generic", view="ide", source=False,
                                       source_dir=None, release=None, build_only=False, daemon=self.endpoint)
        with patch.object(installer, "fetch", side_effect=fetch):
            receipt = installer.install(arguments)
        self.assertEqual(Path(receipt["repo"]), self.project)
        self.assertEqual(Path(receipt["executable"]), self.installed)
        self.assertFalse(receipt["client_configured"])
        self.assertFalse(receipt["mcp_connected"])
        with urllib.request.urlopen(self.endpoint + "/healthz", timeout=5) as response:
            self.assertEqual(Path(json.load(response)["repo"]), self.project)


if __name__ == "__main__":
    unittest.main()
