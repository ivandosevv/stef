#!/usr/bin/env python3
"""Test the POSIX installer against local fixtures, without network or setup."""
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
INSTALLER = ROOT / "install.sh"

FAKE_CURL = r'''#!/bin/sh
set -eu
out=
headers=
url=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output) out=$2; shift 2 ;;
    --dump-header) headers=$2; shift 2 ;;
    --write-out|--proto|--connect-timeout|--max-time|--max-filesize) shift 2 ;;
    --fail|--silent|--show-error) shift ;;
    *) url=$1; shift ;;
  esac
done
printf '%s\n' "$url" >> "$FAKE_REQUESTS"
status=200
printf 'HTTP/1.1 200 OK\r\n\r\n' > "$headers"
case "$url" in
  https://github.com/ivandosevv/stef/releases/latest)
    status=302
    case "$FAKE_LATEST_KIND" in
      external) next=https://outside.invalid/stef.tar.gz ;;
      wrong_project) next=https://github.com/someone/other/releases/tag/v0.3.0 ;;
      no_tag) next=https://github.com/ivandosevv/stef/releases/download/0.3.0 ;;
      *) next=https://github.com/ivandosevv/stef/releases/tag/v0.3.0 ;;
    esac
    printf 'HTTP/1.1 302 Found\r\nLocation: %s\r\n\r\n' "$next" > "$headers"
    : > "$out" ;;
  https://github.com/ivandosevv/stef/releases/tag/v0.3.0)
    printf 'fixture release page' > "$out" ;;
  https://github.com/ivandosevv/stef/releases/download/0.3.0)
    printf 'not a tag' > "$out" ;;
  https://github.com/ivandosevv/stef/releases/download/*/*.sha256)
    [ "$FAKE_FAILURE" != checksum ] || exit 22
    cp "$FAKE_FIXTURES/$(basename "$url")" "$out" ;;
  https://github.com/ivandosevv/stef/releases/download/*/*.tar.gz)
    [ "$FAKE_FAILURE" != archive ] || exit 22
    if [ "$FAKE_ARCHIVE_REDIRECT" = external ]; then
      status=302
      printf 'HTTP/1.1 302 Found\r\nLocation: https://outside.invalid/stef.tar.gz\r\n\r\n' > "$headers"
      : > "$out"
    elif [ "$FAKE_ARCHIVE_REDIRECT" = assets ]; then
      status=302
      printf 'HTTP/1.1 302 Found\r\nLocation: https://release-assets.githubusercontent.com/fixture/stef.tar.gz?sig=value\r\n\r\n' > "$headers"
      : > "$out"
    else cp "$FAKE_FIXTURES/$(basename "$url")" "$out"; fi ;;
  https://release-assets.githubusercontent.com/fixture/stef.tar.gz?sig=value)
    cp "$FAKE_FIXTURES/$FAKE_ARCHIVE_NAME" "$out" ;;
  *) exit 22 ;;
esac
printf '%s %s' "$status" "$url"
'''

FAKE_BINARY = rb'''#!/bin/sh
set -eu
printf '%s\0' "$@" > "$STEF_ARGS_LOG"
printf '{"enabled":true,"fixture":true}\n'
'''


class InstallTests(unittest.TestCase):
    def setUp(self):
        base = ROOT / ".test-workspace"
        base.mkdir(exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(prefix="posix-installer-", dir=base)
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.project = self.root / 'project space " quote $(touch SHOULD_NOT_EXIST)'
        self.project.mkdir()
        self.home = self.root / "home"
        self.home.mkdir()
        self.fixture = self.root / "fixtures"
        self.fixture.mkdir()
        self.path = self.root / "path"
        self.path.mkdir()
        # No Python, Node, Go, make, Git, or host client commands in installer PATH.
        for name in ("awk", "basename", "cat", "chmod", "cmp", "cp", "cut", "date", "gzip",
                     "mkdir", "mktemp", "mv", "rm", "tar", "wc", "sha256sum", "shasum"):
            target = shutil.which(name)
            if target:
                (self.path / name).symlink_to(target)
        (self.path / "curl").write_text(FAKE_CURL)
        (self.path / "curl").chmod(0o755)
        (self.path / "uname").write_text(
            '#!/bin/sh\ncase "$1" in -s) printf "%s\\n" "$FAKE_OS";; '
            '-m) printf "%s\\n" "$FAKE_ARCH";; *) exit 1;; esac\n'
        )
        (self.path / "uname").chmod(0o755)
        self.env = {**os.environ, "PATH": str(self.path), "HOME": str(self.home),
                    "FAKE_FIXTURES": str(self.fixture),
                    "FAKE_REQUESTS": str(self.root / "requests"),
                    "STEF_ARGS_LOG": str(self.root / "args"),
                    "FAKE_OS": "Darwin", "FAKE_ARCH": "arm64",
                    "FAKE_FAILURE": "", "FAKE_LATEST_KIND": "valid",
                    "FAKE_ARCHIVE_REDIRECT": ""}
        self.prepare_archive()

    def prepare_archive(self, *, os_name="darwin", arch="arm64", version="0.3.0",
                        binary_kind="regular", extra=None, optional=True):
        name = f"stef-{version}-{os_name}-{arch}.tar.gz"
        archive = self.fixture / name
        with tarfile.open(archive, "w:gz") as tar:
            def add(member, body, kind=tarfile.REGTYPE, link=""):
                info = tarfile.TarInfo(member)
                info.mode = 0o755 if member == "stef" else 0o644
                info.type = kind
                info.linkname = link
                info.size = len(body) if kind == tarfile.REGTYPE else 0
                tar.addfile(info, io.BytesIO(body) if info.size else None)
            if binary_kind == "symlink":
                add("stef", b"", tarfile.SYMTYPE, "outside")
            elif binary_kind == "hardlink":
                add("stef", b"", tarfile.LNKTYPE, "LICENSE")
            elif binary_kind == "directory":
                add("stef", b"", tarfile.DIRTYPE)
            elif binary_kind != "missing":
                add("stef", b"" if binary_kind == "empty" else FAKE_BINARY)
                if binary_kind == "duplicate":
                    add("stef", FAKE_BINARY)
            if optional:
                add("LICENSE", b"fixture license\n")
                add("skills/stef/SKILL.md", b"fixture skill\n")
            for member, body, kind, link in extra or []:
                add(member, body, kind, link)
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (self.fixture / f"{name}.sha256").write_text(f"{digest}  {name}\n")
        self.env["FAKE_ARCHIVE_NAME"] = name
        self.archive = archive
        self.checksum = self.fixture / f"{name}.sha256"

    def run_install(self, *args, project=None):
        return subprocess.run(["/bin/sh", str(INSTALLER), "--repo",
                               str(project or self.project), "--client", "generic", *args],
                              env=self.env, text=True, capture_output=True, timeout=15)

    def assert_success(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def installed(self):
        return self.project / ".stef/runtime/bin/stef"

    def assert_failed_preserves_binary(self, *args):
        previous = self.installed()
        previous.parent.mkdir(parents=True, exist_ok=True)
        previous.write_bytes(b"previous binary\n")
        previous.chmod(0o755)
        before = previous.stat()
        result = self.run_install(*args)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(previous.read_bytes(), b"previous binary\n")
        self.assertEqual(previous.stat().st_ino, before.st_ino)
        self.assertFalse((self.root / "args").exists())
        self.assertEqual(list(previous.parent.glob(".stef-install-*")), [])
        return result

    def test_latest_installs_and_forwards_arguments_exactly(self):
        result = self.run_install("--view", "native", "--daemon", "http://127.0.0.1:4825")
        self.assert_success(result)
        self.assertEqual(json.loads(result.stdout), {"enabled": True, "fixture": True})
        self.assertEqual((self.root / "args").read_bytes().split(b"\0")[:-1],
                         [s.encode() for s in ("--enable", "--client", "generic",
                          "--view", "native", "--repo", str(self.project),
                          "--daemon", "http://127.0.0.1:4825")])
        self.assertFalse((self.root / "SHOULD_NOT_EXIST").exists())
        self.assertEqual(self.installed().read_bytes(), FAKE_BINARY)
        self.assertEqual((self.project / ".stef/runtime/LICENSE").read_text(), "fixture license\n")
        self.assertEqual((self.project / ".stef/runtime/skills/stef/SKILL.md").read_text(),
                         "fixture skill\n")
        requests = (self.root / "requests").read_text().splitlines()
        self.assertEqual(len(requests), 4)
        self.assertIn("releases/latest", requests[0])

    def test_all_posix_release_targets_build_only(self):
        for os_name, host_os in (("darwin", "Darwin"), ("linux", "Linux")):
            for arch, host_arch in (("amd64", "x86_64"), ("arm64", "aarch64")):
                with self.subTest(os=os_name, arch=arch):
                    self.prepare_archive(os_name=os_name, arch=arch, optional=False)
                    self.env.update(FAKE_OS=host_os, FAKE_ARCH=host_arch)
                    result = self.run_install("--release", "v0.3.0", "--build-only")
                    self.assert_success(result)
                    self.assertEqual(json.loads(result.stdout),
                                     {"installed": True, "install_mode": "release",
                                      "enabled": False, "executable": str(self.installed()),
                                      "repo": str(self.project), "version": "0.3.0"})
                    self.assertFalse((self.root / "args").exists())
        self.assertNotIn("releases/latest", (self.root / "requests").read_text())

    def test_identical_binary_preserves_inode_and_mtime(self):
        self.assert_success(self.run_install("--build-only"))
        before = self.installed().stat()
        self.assert_success(self.run_install("--build-only"))
        after = self.installed().stat()
        self.assertEqual((after.st_ino, after.st_mtime_ns), (before.st_ino, before.st_mtime_ns))

    def test_download_failures_preserve_installed_binary(self):
        for failure in ("checksum", "archive"):
            with self.subTest(failure=failure):
                self.env["FAKE_FAILURE"] = failure
                self.assert_failed_preserves_binary()

    def test_hash_failure_preserves_installed_binary(self):
        self.checksum.write_text(f"{'0' * 64}  {self.archive.name}\n")
        self.assertIn("checksum mismatch", self.assert_failed_preserves_binary().stderr)

    def test_selected_member_and_checksum_size_limits(self):
        self.prepare_archive(optional=False,
                             extra=[("LICENSE", b"x" * 1048577, tarfile.REGTYPE, "")])
        self.assert_failed_preserves_binary()
        self.checksum.write_bytes(b"x" * 4097)
        # Some shells enforce ulimit during download; others reach the byte check.
        self.assert_failed_preserves_binary()

    def test_standard_hash_tools_are_supported_independently(self):
        for name in ("sha256sum", "shasum"):
            if not (self.path / name).exists():
                continue
            with self.subTest(tool=name):
                other = self.path / ("shasum" if name == "sha256sum" else "sha256sum")
                target = other.readlink() if other.exists() else None
                if target:
                    other.unlink()
                try:
                    self.assert_success(self.run_install("--build-only"))
                finally:
                    if target:
                        other.symlink_to(target)

    def test_unsupported_os_and_architecture_fail_before_network(self):
        for os_name, arch in (("Windows_NT", "x86_64"), ("Linux", "i686")):
            with self.subTest(os=os_name, arch=arch):
                self.env.update(FAKE_OS=os_name, FAKE_ARCH=arch)
                self.assert_failed_preserves_binary()
        self.assertFalse((self.root / "requests").exists())

    def test_nonregular_duplicate_missing_empty_binary_rejected(self):
        for kind in ("symlink", "hardlink", "directory", "duplicate", "missing", "empty"):
            with self.subTest(kind=kind):
                self.prepare_archive(binary_kind=kind)
                self.assert_failed_preserves_binary()

    def test_optional_selected_members_must_be_unique_regular_files(self):
        for member in ("LICENSE", "skills/stef/SKILL.md"):
            for kind in (tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.REGTYPE):
                with self.subTest(member=member, kind=kind):
                    self.prepare_archive(extra=[(member, b"duplicate", kind, "stef")])
                    self.assert_failed_preserves_binary()

    def test_unselected_traversal_and_links_never_extracted(self):
        outside = self.root / "outside"
        outside.write_text("preserve")
        self.prepare_archive(extra=[(str(outside), b"overwrite", tarfile.REGTYPE, ""),
                             ("../../../../../../outside", b"escape", tarfile.REGTYPE, ""),
                             ("README.md", b"", tarfile.SYMTYPE, str(outside))])
        self.assert_success(self.run_install("--build-only"))
        self.assertEqual(outside.read_text(), "preserve")
        self.assertFalse((self.project / ".stef/runtime/README.md").exists())

    def test_home_root_and_escaping_runtime_links_rejected(self):
        for project in (self.home, Path("/")):
            with self.subTest(project=project):
                result = self.run_install(project=project)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("home folder or a filesystem root", result.stderr)
        outside = self.root / "outside-runtime"
        outside.mkdir()
        for member in (".stef", ".stef/runtime", ".stef/runtime/bin",
                       ".stef/runtime/skills", ".stef/runtime/skills/stef"):
            with self.subTest(member=member):
                target = self.project / member
                target.parent.mkdir(parents=True, exist_ok=True)
                target.symlink_to(outside, target_is_directory=True)
                result = self.run_install("--build-only")
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertIn("directory link", result.stderr)
                target.unlink()
        self.assertEqual(list(outside.iterdir()), [])

    def test_existing_runtime_file_links_rejected(self):
        outside = self.root / "outside"
        outside.write_text("preserve")
        for member in ("bin/stef", "LICENSE", "skills/stef/SKILL.md"):
            with self.subTest(member=member):
                target = self.project / ".stef/runtime" / member
                target.parent.mkdir(parents=True, exist_ok=True)
                target.symlink_to(outside)
                result = self.run_install("--build-only")
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("runtime files cannot be links", result.stderr)
                target.unlink()
        self.assertEqual(outside.read_text(), "preserve")

    def test_external_redirect_rejected_before_host_contacted(self):
        for kind in ("external", "wrong_project"):
            with self.subTest(kind=kind):
                self.env["FAKE_LATEST_KIND"] = kind
                result = self.assert_failed_preserves_binary()
                self.assertIn("release", result.stderr)
        self.env["FAKE_LATEST_KIND"] = "valid"
        self.env["FAKE_ARCHIVE_REDIRECT"] = "external"
        self.assert_failed_preserves_binary()
        requests = (self.root / "requests").read_text()
        self.assertNotIn("outside.invalid", requests)
        self.assertNotIn("someone/other", requests)

    def test_release_assets_redirect_supported(self):
        self.env["FAKE_ARCHIVE_REDIRECT"] = "assets"
        self.assert_success(self.run_install("--build-only"))
        self.assertIn("release-assets.githubusercontent.com", (self.root / "requests").read_text())

    def test_malformed_checksum_and_version_rejected(self):
        valid = self.checksum.read_text()
        for body in ("invalid\n", valid + valid, valid + "\n",
                     valid.replace(self.archive.name, "different.tar.gz")):
            with self.subTest(checksum=body):
                self.checksum.write_text(body)
                self.assertIn("SHA256 file is invalid", self.assert_failed_preserves_binary().stderr)
        for version in ("../../invalid", "0.3.0\n0.3.1", "0.3.0?query"):
            with self.subTest(version=version):
                self.assertIn("release version",
                              self.assert_failed_preserves_binary("--release", version).stderr)
        self.env["FAKE_LATEST_KIND"] = "no_tag"
        self.assertIn("release tag", self.assert_failed_preserves_binary().stderr)

    def test_project_and_agent_required(self):
        for args in ([], ["--repo", str(self.project)],
                     ["--repo", "relative", "--client", "generic"],
                     ["--repo", str(self.project), "--client", "generic", "--view", "browser"]):
            with self.subTest(args=args):
                result = subprocess.run(["/bin/sh", str(INSTALLER), *args], env=self.env,
                                        text=True, capture_output=True, timeout=15)
                self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "requests").exists())


if __name__ == "__main__":
    unittest.main()
