"""Package a built stef executable for macOS, Linux, or Windows."""
import argparse
import hashlib
from pathlib import Path
import platform
import re
import struct
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parent.parent
SUPPORTED_OS = {"darwin", "linux", "windows"}
SUPPORTED_ARCH = {"amd64", "arm64"}
NL = chr(10)

def host_target():
    system = {"Darwin": "darwin", "Linux": "linux", "Windows": "windows"}.get(platform.system())
    arch = {"arm64": "arm64", "aarch64": "arm64", "amd64": "amd64", "x86_64": "amd64"}.get(platform.machine().lower())
    return system, arch

def validate_binary(binary, system, arch):
    """Read headers so an archive cannot silently mislabel its target."""
    with binary.open("rb") as source:
        header = source.read(64)
        matches = False
        if system == "linux" and len(header) >= 20 and header[:6] == bytes.fromhex("7f454c460201"):
            matches = struct.unpack_from("<H", header, 18)[0] == {"amd64": 62, "arm64": 183}[arch]
        elif system == "darwin" and len(header) >= 8 and header[:4] == bytes.fromhex("cffaedfe"):
            matches = struct.unpack_from("<I", header, 4)[0] == {"amd64": 0x1000007, "arm64": 0x100000C}[arch]
        elif system == "windows" and len(header) == 64 and header[:2] == b"MZ":
            offset = struct.unpack_from("<I", header, 60)[0]
            if 64 <= offset <= 1024 * 1024:
                source.seek(offset)
                pe = source.read(6)
                matches = len(pe) == 6 and pe[:4] == bytes.fromhex("50450000") and struct.unpack_from("<H", pe, 4)[0] == {"amd64": 0x8664, "arm64": 0xAA64}[arch]
        if not matches:
            raise ValueError(f"Executable does not match {system}/{arch}: {binary}")

def package(root, binary, output, version, system, arch):
    if system not in SUPPORTED_OS or arch not in SUPPORTED_ARCH:
        raise ValueError("Supported targets: darwin, linux, windows; amd64 or arm64")
    if not re.fullmatch(r"[0-9]+[.][0-9]+[.][0-9]+(?:-[A-Za-z0-9.-]+)?", version):
        raise ValueError("Use a version such as 0.3.0 or 0.3.0-beta.1")
    if not binary.is_file() or binary.is_symlink():
        raise ValueError(f"Build stef first; regular executable missing: {binary}")
    validate_binary(binary, system, arch)
    files = [(binary, "stef.exe" if system == "windows" else "stef")]
    for name in ("LICENSE", "README.md", "INSTALL.md"):
        source = root / name
        if source.is_symlink():
            raise ValueError(f"Release guidance cannot contain symlinks: {source}")
        if source.is_file():
            files.append((source, name))
        elif name == "LICENSE":
            raise ValueError("Release needs its LICENSE")
    skill = root / ".agents/skills/stef"
    for source in sorted(skill.rglob("*")):
        if source.is_symlink():
            raise ValueError(f"Release skill cannot contain symlinks: {source}")
        if source.is_file():
            files.append((source, "skills/stef/" + source.relative_to(skill).as_posix()))
    if not (skill / "SKILL.md").is_file():
        raise ValueError("Release needs the portable stef skill")
    output.mkdir(parents=True, exist_ok=True)
    suffix = ".zip" if system == "windows" else ".tar.gz"
    archive = output / f"stef-{version}-{system}-{arch}{suffix}"
    temporary = archive.with_name(archive.name + ".tmp")
    try:
        if system == "windows":
            with zipfile.ZipFile(temporary, "w", compression=zipfile.ZIP_DEFLATED) as bundle:
                for source, name in files:
                    bundle.write(source, name)
        else:
            with tarfile.open(temporary, "w:gz") as bundle:
                for source, name in files:
                    info = bundle.gettarinfo(str(source), arcname=name)
                    info.uid = info.gid = 0
                    info.uname = info.gname = ""
                    info.mode = 0o755 if name == "stef" else 0o644
                    with source.open("rb") as content:
                        bundle.addfile(info, content)
        temporary.replace(archive)
    finally:
        temporary.unlink(missing_ok=True)
    checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
    archive.with_name(archive.name + ".sha256").write_text(f"{checksum}  {archive.name}" + NL, encoding="utf-8")
    checksums = output / "checksums.txt"
    entries = {}
    if checksums.exists():
        for line in checksums.read_text(encoding="utf-8").splitlines():
            digest, name = line.split("  ", 1)
            entries[name] = digest
    entries[archive.name] = checksum
    checksums.write_text("".join(f"{entries[name]}  {name}" + NL for name in sorted(entries)), encoding="utf-8")
    return archive, checksum

def brew_formula(root, archive, checksum, version, url_prefix):
    url = url_prefix.rstrip("/") + "/" + archive.name if url_prefix else archive.as_uri()
    formula = root / "packaging/homebrew/Formula/stef.rb"
    formula.parent.mkdir(parents=True, exist_ok=True)
    formula.write_text(f"""# Generated by scripts/package-release.py; verify the published URL before use.
class Stef < Formula
  desc "Interactive local companion for coding agents"
  homepage "https://github.com/ivandosevv/stef"
  url "{url}"
  version "{version}"
  sha256 "{checksum}"
  license "MIT"

  depends_on :macos
  depends_on arch: :arm64

  def install
    bin.install "stef"
    pkgshare.install "skills"
  end

  test do
    assert_match "Run stdio MCP", shell_output("#{{bin}}/stef --help 2>&1")
  end
end
""", encoding="utf-8")
    return formula

def main():
    system, arch = host_target()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--goos", default=system, choices=sorted(SUPPORTED_OS))
    parser.add_argument("--goarch", default=arch, choices=sorted(SUPPORTED_ARCH))
    parser.add_argument("--version", default="0.3.0")
    parser.add_argument("--binary", type=Path, help="Built executable; defaults to bin/stef or bin/stef.exe")
    parser.add_argument("--output-dir", type=Path, default=ROOT / "artifacts/releases")
    parser.add_argument("--brew-formula", action="store_true", help="Also write the optional macOS ARM64 draft recipe")
    parser.add_argument("--url-prefix", help="Published archive base URL for --brew-formula")
    args = parser.parse_args()
    if args.brew_formula and (args.goos, args.goarch) != ("darwin", "arm64"):
        parser.error("The optional Brew recipe is for darwin/arm64")
    binary = args.binary or ROOT / "bin" / ("stef.exe" if args.goos == "windows" else "stef")
    try:
        archive, checksum = package(ROOT, binary, args.output_dir.resolve(), args.version, args.goos, args.goarch)
        print(f"Archive: {archive}")
        print(f"SHA256: {checksum}")
        if args.brew_formula:
            print(f"Draft Brew recipe: {brew_formula(ROOT, archive, checksum, args.version, args.url_prefix)}")
    except (ValueError, OSError) as error:
        parser.exit(1, str(error) + NL)

if __name__ == "__main__":
    main()
