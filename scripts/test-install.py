"""Exercise portable installer boundaries without network, compilers or client profiles."""
import argparse
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import warnings
import zipfile

ROOT = Path(__file__).resolve().parent.parent


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value


installer = module("stef_installer", ROOT / "install.py")
builder = module("stef_builder", ROOT / "scripts/build.py")


def archive(entries, extension="tar.gz"):
    output = io.BytesIO()
    if extension == "zip":
        with warnings.catch_warnings(), zipfile.ZipFile(output, "w") as bundle:
            warnings.simplefilter("ignore", UserWarning)
            for name, data, kind in entries:
                info = zipfile.ZipInfo(name)
                info.external_attr = (0o120777 if kind == "link" else 0o100644) << 16
                bundle.writestr(info, data)
    else:
        with tarfile.open(fileobj=output, mode="w:gz") as bundle:
            for name, data, kind in entries:
                info = tarfile.TarInfo(name)
                if kind == "link":
                    info.type, info.linkname = tarfile.SYMTYPE, "../outside"
                    bundle.addfile(info)
                else:
                    info.size = len(data)
                    bundle.addfile(info, io.BytesIO(data))
    return output.getvalue()


class InstallerTest(unittest.TestCase):
    def setUp(self):
        (ROOT / ".test-workspace").mkdir(exist_ok=True)
        self.temporary = tempfile.TemporaryDirectory(dir=ROOT / ".test-workspace", prefix="install.")
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        self.project = self.base / "Current project $() & spaces"
        self.project.mkdir()

    def arguments(self, **changes):
        arguments = dict(repo=str(self.project), client="opencode", view="ide", source_dir=None,
                         source=False, release=None, build_only=True, daemon=None)
        arguments.update(changes)
        return argparse.Namespace(**arguments)

    def published(self, payload, goos="linux", goarch="amd64", *, checksum=None):
        extension = "zip" if goos == "windows" else "tar.gz"
        name = "stef-0.3.0-" + goos + "-" + goarch + "." + extension
        url = installer.REPOSITORY + "/releases/download/v0.3.0/"
        document = {"tag_name": "v0.3.0", "draft": False, "prerelease": False, "assets": [
            {"name": name, "browser_download_url": url + name},
            {"name": name + ".sha256", "browser_download_url": url + name + ".sha256"}]}
        digest = checksum or hashlib.sha256(payload).hexdigest()
        values = {installer.API + "/latest": json.dumps(document).encode(),
                  url + name: payload, url + name + ".sha256": (digest + "  " + name + "\n").encode()}
        return lambda address, maximum: values[address]

    def test_release_targets_cover_both_architectures_and_three_systems(self):
        for system, machine, expected in [("Darwin", "arm64", ("darwin", "arm64")),
                                          ("Darwin", "x86_64", ("darwin", "amd64")),
                                          ("Windows", "AMD64", ("windows", "amd64")),
                                          ("Windows", "ARM64", ("windows", "arm64")),
                                          ("Linux", "x86_64", ("linux", "amd64")),
                                          ("Linux", "aarch64", ("linux", "arm64"))]:
            self.assertEqual(installer.target(system, machine), expected)
            self.assertEqual(builder.platform_target(system, machine), expected)
        with self.assertRaises(installer.InstallError):
            installer.target("Windows", "x86")

    def test_archive_installs_without_build_tools_and_uses_one_stable_path(self):
        for goos, extension in [("linux", "tar.gz"), ("windows", "zip")]:
            with self.subTest(goos=goos):
                name = "stef.exe" if goos == "windows" else "stef"
                payload = archive([(name, b"verified binary", "file"), ("LICENSE", b"license", "file")], extension)
                with patch.object(installer, "target", return_value=(goos, "amd64")), \
                     patch.object(installer, "fetch", side_effect=self.published(payload, goos)), \
                     patch.object(installer, "source_tools") as tools, patch.object(installer.subprocess, "run") as commands:
                    receipt = installer.install(self.arguments())
                    tools.assert_not_called()
                    commands.assert_not_called()
                binary = self.project / ".stef/runtime/bin" / name
                self.assertEqual(receipt["executable"], str(binary))
                self.assertEqual(binary.read_bytes(), b"verified binary")
                self.assertEqual((self.project / ".stef/runtime/LICENSE").read_bytes(), b"license")
                self.assertFalse(receipt["enabled"])

    def test_enable_runs_in_the_original_project_with_literal_arguments(self):
        payload = archive([("stef", b"binary", "file"), ("LICENSE", b"license", "file")])
        result = subprocess.CompletedProcess([], 0, stdout=json.dumps({"client_configured": False, "mcp_connected": False, "view_opened": False}), stderr="")
        with patch.object(installer, "target", return_value=("linux", "amd64")), \
             patch.object(installer, "fetch", side_effect=self.published(payload)), \
             patch.object(installer.subprocess, "run", return_value=result) as command:
            receipt = installer.install(self.arguments(build_only=False, client="generic", daemon="http://127.0.0.1:4980"))
        expected = [str(self.project / ".stef/runtime/bin/stef"), "--enable", "--client", "generic", "--view", "ide",
                    "--repo", str(self.project), "--daemon", "http://127.0.0.1:4980"]
        self.assertEqual(command.call_args.args[0], expected)
        self.assertEqual(command.call_args.kwargs["cwd"], self.project)
        self.assertFalse(receipt["client_configured"])
        self.assertFalse(receipt["mcp_connected"])
        self.assertFalse(receipt["view_opened"])

    def test_home_root_and_escaping_runtime_fail_before_download_or_setup(self):
        for path in [Path.home(), Path(self.project.anchor)]:
            with self.subTest(path=path), patch.object(installer, "fetch") as fetch, self.assertRaises(installer.InstallError):
                installer.install(self.arguments(repo=str(path)))
            fetch.assert_not_called()
        outside = self.base / "outside"
        outside.mkdir()
        try:
            (self.project / ".stef").symlink_to(outside, target_is_directory=True)
        except OSError as error:
            self.skipTest(str(error))
        with patch.object(installer, "fetch") as fetch, self.assertRaises(installer.InstallError):
            installer.install(self.arguments())
        fetch.assert_not_called()
        self.assertEqual(list(outside.iterdir()), [])

    def test_checksum_failure_keeps_the_previous_binary_and_never_falls_back(self):
        binary = self.project / ".stef/runtime/bin/stef"
        binary.parent.mkdir(parents=True)
        binary.write_bytes(b"previous working binary")
        payload = archive([("stef", b"new binary", "file"), ("LICENSE", b"license", "file")])
        with patch.object(installer, "target", return_value=("linux", "amd64")), \
             patch.object(installer, "fetch", side_effect=self.published(payload, checksum="0" * 64)), \
             patch.object(installer, "source_tools") as tools, self.assertRaisesRegex(installer.InstallError, "checksum"):
            installer.install(self.arguments())
        self.assertEqual(binary.read_bytes(), b"previous working binary")
        tools.assert_not_called()

    def test_only_missing_latest_release_allows_source_fallback(self):
        with patch.object(installer, "fetch", side_effect=installer.NoRelease):
            self.assertIsNone(installer.release_payload("latest", "linux", "amd64"))
            with self.assertRaises(installer.InstallError):
                installer.release_payload("0.3.0", "linux", "amd64")
        document = {"tag_name": "v0.3.0", "assets": [{"name": "stef-0.3.0-linux-amd64.tar.gz"}]}
        with patch.object(installer, "fetch", return_value=json.dumps(document).encode()), self.assertRaises(installer.InstallError):
            installer.release_payload("latest", "linux", "amd64")

    def test_release_asset_urls_cannot_escape_the_repository(self):
        for url in ["https://example.com/stef.zip", installer.REPOSITORY + "/releases/download/v0.3.0/other.zip",
                    installer.REPOSITORY + "/releases/download/v0.3.0/stef.zip?redirect=1"]:
            with self.subTest(url=url), self.assertRaises(installer.InstallError):
                installer.asset_url({"name": "stef.zip", "browser_download_url": url}, "v0.3.0")

    def test_archive_paths_links_and_duplicate_names_are_rejected(self):
        for extension in ["tar.gz", "zip"]:
            for entries in [[("../escape", b"bad", "file")], [("/absolute", b"bad", "file")],
                            [("directory\\escape", b"bad", "file")], [("NUL", b"bad", "file")],
                            [("linked", b"../outside", "link")],
                            [("stef", b"a", "file"), ("STEF", b"b", "file")]]:
                with self.subTest(extension=extension, entries=entries):
                    output = self.base / ("archive-" + next(tempfile._get_candidate_names()))
                    output.mkdir()
                    with self.assertRaises(installer.InstallError):
                        installer.extract_archive(archive(entries, extension), extension, output)
        self.assertFalse((self.base / "escape").exists())

    def test_locked_updates_preserve_old_binary_and_identical_reinstall_is_local(self):
        source = self.base / "compiled"
        source.write_bytes(b"new")
        old = self.project / ".stef/runtime/bin/stef.exe"
        old.parent.mkdir(parents=True)
        old.write_bytes(b"old")
        with patch.object(installer.os, "replace", side_effect=PermissionError), self.assertRaises(installer.InstallError):
            installer.install_binary(self.project, source, "windows")
        self.assertEqual(old.read_bytes(), b"old")
        self.assertEqual(list(old.parent.iterdir()), [old])
        source.write_bytes(b"old")
        with patch.object(installer.os, "replace") as replace:
            self.assertEqual(installer.install_binary(self.project, source, "windows"), old)
            replace.assert_not_called()

    def test_failed_compile_preserves_the_working_binary(self):
        source = self.base / "stef source"
        (source / "daemon").mkdir(parents=True)
        (source / "ui").mkdir()
        (source / "daemon/go.mod").touch()
        (source / "ui/package-lock.json").touch()
        binary = source / "bin/stef.exe"
        binary.parent.mkdir()
        binary.write_bytes(b"working")
        calls = []
        def run(arguments, **kwargs):
            calls.append((arguments, kwargs))
            if arguments[:2] == ["go", "build"]:
                raise subprocess.CalledProcessError(1, arguments)
        with patch.object(builder, "prerequisites", return_value=("go", ["npm.cmd"])), \
             patch.object(builder.subprocess, "run", side_effect=run), self.assertRaises(subprocess.CalledProcessError):
            builder.build(source, goos="windows", goarch="arm64")
        self.assertEqual(binary.read_bytes(), b"working")
        self.assertEqual(list(binary.parent.iterdir()), [binary])
        self.assertTrue(all(kwargs["env"]["CGO_ENABLED"] == "0" for _, kwargs in calls))
        self.assertTrue(all(kwargs["env"]["GOOS"] == "windows" for _, kwargs in calls))

    def test_escaping_build_output_fails_before_installing_dependencies(self):
        source = self.base / "stef source"
        (source / "daemon").mkdir(parents=True)
        (source / "ui").mkdir()
        (source / "daemon/go.mod").touch()
        (source / "ui/package-lock.json").touch()
        with patch.object(builder, "prerequisites") as tools, self.assertRaises(builder.BuildError):
            builder.build(source, output=self.base / "outside")
        tools.assert_not_called()


if __name__ == "__main__":
    unittest.main()
