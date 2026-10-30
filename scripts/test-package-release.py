"""Check release contents, checksums, and platform labels without publishing."""
import hashlib
import importlib.util
from pathlib import Path
import struct
import tarfile
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location("stef_package_release", ROOT / "scripts/package-release.py")
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


def executable_header(system, arch):
    data = bytearray(128)
    if system == "linux":
        data[:6] = bytes.fromhex("7f454c460201")
        struct.pack_into("<H", data, 18, 62 if arch == "amd64" else 183)
    elif system == "darwin":
        data[:4] = bytes.fromhex("cffaedfe")
        struct.pack_into("<I", data, 4, 0x1000007 if arch == "amd64" else 0x100000C)
    else:
        data[:2] = b"MZ"
        struct.pack_into("<I", data, 60, 64)
        data[64:68] = bytes.fromhex("50450000")
        struct.pack_into("<H", data, 68, 0x8664 if arch == "amd64" else 0xAA64)
    return bytes(data)


class ReleaseTest(unittest.TestCase):
    def setUp(self):
        (ROOT / ".test-workspace").mkdir(exist_ok=True)
        self.temp = tempfile.TemporaryDirectory(dir=ROOT / ".test-workspace", prefix="package.")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.output = self.root / "output"
        self.binary = self.root / "built executable"
        self.skill = self.root / ".agents/skills/stef"
        self.skill.mkdir(parents=True)
        (self.skill / "SKILL.md").write_text("Enable stef in this project.", encoding="utf-8")
        for name in ("LICENSE", "README.md", "INSTALL.md"):
            (self.root / name).write_text(name, encoding="utf-8")
        (self.root / "private-settings.json").write_text("Do not bundle this.", encoding="utf-8")

    def package(self, system, arch, version="0.3.0"):
        self.binary.write_bytes(executable_header(system, arch))
        return release.package(self.root, self.binary, self.output, version, system, arch)

    def test_six_targets_include_only_runtime_and_public_guidance(self):
        expected = {"LICENSE", "README.md", "INSTALL.md", "skills/stef/SKILL.md"}
        for system in ("darwin", "linux", "windows"):
            for arch in ("amd64", "arm64"):
                with self.subTest(system=system, arch=arch):
                    archive, checksum = self.package(system, arch)
                    executable = "stef.exe" if system == "windows" else "stef"
                    suffix = ".zip" if system == "windows" else ".tar.gz"
                    self.assertEqual(archive.name, f"stef-0.3.0-{system}-{arch}{suffix}")
                    if system == "windows":
                        with zipfile.ZipFile(archive) as bundle:
                            self.assertEqual(set(bundle.namelist()), expected | {executable})
                            self.assertEqual(bundle.read(executable), self.binary.read_bytes())
                    else:
                        with tarfile.open(archive) as bundle:
                            self.assertEqual(set(bundle.getnames()), expected | {executable})
                            self.assertTrue(all(member.isfile() for member in bundle.getmembers()))
                            self.assertEqual(bundle.getmember(executable).mode, 0o755)
                            self.assertEqual(bundle.extractfile(executable).read(), self.binary.read_bytes())
                    self.assertEqual(checksum, hashlib.sha256(archive.read_bytes()).hexdigest())
                    self.assertEqual(archive.with_name(archive.name + ".sha256").read_text().splitlines(), [f"{checksum}  {archive.name}"])
        lines = (self.output / "checksums.txt").read_text().splitlines()
        self.assertEqual(len(lines), 6)
        self.assertEqual(lines, sorted(lines, key=lambda line: line.split("  ", 1)[1]))

    def test_mislabeled_executable_preserves_existing_archive(self):
        archive, _ = self.package("linux", "arm64")
        old = archive.read_bytes()
        self.binary.write_bytes(executable_header("windows", "arm64"))
        with self.assertRaisesRegex(ValueError, "does not match"):
            release.package(self.root, self.binary, self.output, "0.3.0", "linux", "arm64")
        self.assertEqual(archive.read_bytes(), old)
        self.binary.write_bytes(executable_header("linux", "amd64"))
        with self.assertRaisesRegex(ValueError, "does not match"):
            release.package(self.root, self.binary, self.output, "0.3.0", "linux", "arm64")
        self.assertEqual(archive.read_bytes(), old)

    def test_invalid_version_cannot_escape_output(self):
        for version in ("../outside", "0.3.0/other", "v0.3.0"):
            with self.subTest(version=version), self.assertRaises(ValueError):
                self.package("darwin", "arm64", version)
        self.assertFalse(self.output.exists())

    def test_skill_links_are_rejected_before_writing_an_archive(self):
        try:
            (self.skill / "linked-secret.md").symlink_to(self.root / "private-settings.json")
        except OSError as error:
            self.skipTest(str(error))
        with self.assertRaisesRegex(ValueError, "symlinks"):
            self.package("darwin", "arm64")
        self.assertFalse(self.output.exists())

    def test_repack_replaces_one_checksum_without_discarding_other_targets(self):
        first, _ = self.package("linux", "amd64")
        second, _ = self.package("windows", "arm64")
        self.binary.write_bytes(executable_header("linux", "amd64") + b"new content")
        _, digest = release.package(self.root, self.binary, self.output, "0.3.0", "linux", "amd64")
        lines = (self.output / "checksums.txt").read_text().splitlines()
        self.assertEqual(len(lines), 2)
        self.assertIn(f"{digest}  {first.name}", lines)
        self.assertTrue(any(line.endswith("  " + second.name) for line in lines))


if __name__ == "__main__":
    unittest.main()
