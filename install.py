"""Install and enable stef in the project where the user is already working."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile


REPOSITORY = "https://github.com/ivandosevv/stef"
API = "https://api.github.com/repos/ivandosevv/stef/releases"
MAX_ARCHIVE = 100 * 1024 * 1024
MAX_EXTRACTED = 200 * 1024 * 1024
DOWNLOAD_HOSTS = {"github.com", "api.github.com", "release-assets.githubusercontent.com",
                  "objects.githubusercontent.com", "github-releases.githubusercontent.com"}


class InstallError(Exception):
    pass


class NoRelease(Exception):
    pass


def project_root(value):
    path = Path(value).expanduser()
    if not path.is_absolute():
        raise InstallError("Use an existing absolute --repo for the project you are working on.")
    path = path.resolve(strict=True)
    if not path.is_dir() or path.parent == path or path == Path.home().resolve():
        raise InstallError("Choose your project folder, not your home folder or a filesystem root.")
    return path


def confined(project, path):
    resolved = path.resolve()
    if project not in resolved.parents:
        raise InstallError("The stef runtime escapes the project through a symlink; keep .stef inside your project.")
    return resolved


def target(system=None, machine=None):
    system = platform.system() if system is None else system
    machine = platform.machine() if machine is None else machine
    goos = {"Darwin": "darwin", "Windows": "windows", "Linux": "linux"}.get(system)
    goarch = {"x86_64": "amd64", "AMD64": "amd64", "amd64": "amd64",
              "arm64": "arm64", "aarch64": "arm64", "ARM64": "arm64"}.get(machine)
    if not goos or not goarch:
        raise InstallError("stef supports macOS, Windows and Linux on x64 or ARM64.")
    return goos, goarch


class SafeRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, response, code, message, headers, newurl):
        parsed = urllib.parse.urlparse(newurl)
        if parsed.scheme != "https" or parsed.hostname not in DOWNLOAD_HOSTS or parsed.username:
            raise InstallError("GitHub redirected the download outside its HTTPS release hosts.")
        return super().redirect_request(request, response, code, message, headers, newurl)


def fetch(url, maximum):
    request = urllib.request.Request(url, headers={"User-Agent": "stef-installer", "Accept": "application/vnd.github+json"})
    opener = urllib.request.build_opener(SafeRedirect())
    deadline = time.monotonic() + 60
    try:
        with opener.open(request, timeout=60) as response:
            length = response.headers.get("Content-Length")
            if length and int(length) > maximum:
                raise InstallError("GitHub download exceeds the supported size.")
            chunks, total = [], 0
            while True:
                if time.monotonic() > deadline:
                    raise InstallError("GitHub download timed out; retry or use --source.")
                chunk = response.read(min(65536, maximum - total + 1))
                if time.monotonic() > deadline:
                    raise InstallError("GitHub download timed out; retry or use --source.")
                if not chunk:
                    return b"".join(chunks)
                total += len(chunk)
                if total > maximum:
                    raise InstallError("GitHub download exceeds the supported size.")
                chunks.append(chunk)
    except urllib.error.HTTPError as error:
        if error.code == 404:
            raise NoRelease() from error
        raise InstallError("GitHub download returned HTTP " + str(error.code) + "; retry or use --source.") from error
    except urllib.error.URLError as error:
        raise InstallError("Cannot reach GitHub releases; retry or use --source.") from error


def release_version(tag):
    if not isinstance(tag, str):
        raise InstallError("GitHub returned an invalid release version.")
    match = re.fullmatch(r"v?([0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?)", tag)
    if not match:
        raise InstallError("Use latest or a stef release version such as 0.3.0.")
    return match.group(1)


def asset_url(asset, tag):
    url = asset.get("browser_download_url", "")
    parsed = urllib.parse.urlparse(url)
    expected = "/ivandosevv/stef/releases/download/" + tag + "/" + asset["name"]
    if (parsed.scheme != "https" or parsed.hostname != "github.com" or parsed.username
            or parsed.query or parsed.fragment or urllib.parse.unquote(parsed.path) != expected):
        raise InstallError("Release asset does not belong to the stef GitHub repository.")
    return url


def release_payload(selection, goos, goarch):
    suffix = "/latest" if selection == "latest" else "/tags/v" + release_version(selection)
    try:
        document = json.loads(fetch(API + suffix, 2 * 1024 * 1024))
    except NoRelease:
        if selection != "latest":
            raise InstallError("The requested stef release was not found.")
        return None
    except (ValueError, TypeError) as error:
        raise InstallError("GitHub returned invalid release metadata.") from error
    if not isinstance(document, dict) or document.get("draft") or document.get("prerelease"):
        raise InstallError("Use a published stable stef release.")
    tag = document.get("tag_name", "")
    version = release_version(tag)
    extension = "zip" if goos == "windows" else "tar.gz"
    name = "stef-" + version + "-" + goos + "-" + goarch + "." + extension
    assets = document.get("assets", [])
    if not isinstance(assets, list):
        raise InstallError("GitHub returned invalid release assets.")
    matches = [asset for asset in assets if isinstance(asset, dict) and asset.get("name") == name]
    checks = [asset for asset in assets if isinstance(asset, dict) and asset.get("name") == name + ".sha256"]
    if not matches and selection == "latest":
        return None
    if len(matches) != 1 or len(checks) != 1:
        raise InstallError("The release needs one matching archive and its SHA256 file.")
    try:
        checksum = fetch(asset_url(checks[0], tag), 4096).decode("ascii").strip()
        match = re.fullmatch(r"([a-fA-F0-9]{64})\s+\*?" + re.escape(name), checksum)
        if not match:
            raise InstallError("The release SHA256 file is invalid.")
        archive = fetch(asset_url(matches[0], tag), MAX_ARCHIVE)
    except NoRelease as error:
        raise InstallError("A published release asset is missing; retry or use --source.") from error
    if hashlib.sha256(archive).hexdigest() != match.group(1).lower():
        raise InstallError("Release checksum mismatch. Your existing stef was preserved.")
    return archive, extension, version


def extract_archive(data, extension, directory):
    seen, total = set(), 0
    def destination(name, size):
        nonlocal total
        if "\\" in name or "\x00" in name:
            raise InstallError("Unsafe path in the release archive.")
        path = PurePosixPath(name)
        if path.is_absolute() or ".." in path.parts or "." in path.parts or not path.parts:
            raise InstallError("Unsafe path in the release archive.")
        if any(":" in part or part.rstrip(". ") != part or re.match(r"(?i)^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)", part) for part in path.parts):
            raise InstallError("Unsafe path in the release archive.")
        key = str(path).casefold()
        total += size
        if key in seen or len(seen) >= 512 or size > 64 * 1024 * 1024 or total > MAX_EXTRACTED:
            raise InstallError("Duplicate or oversized files in the release archive.")
        seen.add(key)
        output = directory.joinpath(*path.parts)
        if directory not in output.resolve().parents:
            raise InstallError("Unsafe path in the release archive.")
        return output
    def write(source, output, size):
        output.parent.mkdir(parents=True, exist_ok=True)
        with output.open("xb") as file:
            remaining = size
            while remaining:
                chunk = source.read(min(remaining, 65536))
                if not chunk:
                    raise InstallError("Incomplete file in the release archive.")
                file.write(chunk)
                remaining -= len(chunk)
    if extension == "zip":
        with zipfile.ZipFile(io.BytesIO(data)) as archive:
            for entry in archive.infolist():
                mode = entry.external_attr >> 16
                if stat.S_ISLNK(mode) or (stat.S_IFMT(mode) and not (stat.S_ISREG(mode) or stat.S_ISDIR(mode))):
                    raise InstallError("Release archives cannot contain links or special files.")
                output = destination(entry.filename, entry.file_size)
                if entry.is_dir():
                    output.mkdir(parents=True, exist_ok=True)
                else:
                    with archive.open(entry) as stream:
                        write(stream, output, entry.file_size)
    else:
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
            for entry in archive:
                if not entry.isfile() and not entry.isdir():
                    raise InstallError("Release archives cannot contain links or special files.")
                output = destination(entry.name, entry.size)
                if entry.isdir():
                    output.mkdir(parents=True, exist_ok=True)
                else:
                    with archive.extractfile(entry) as stream:
                        write(stream, output, entry.size)


def install_binary(project, source, goos):
    directory = confined(project, project / ".stef/runtime/bin")
    directory.mkdir(parents=True, exist_ok=True)
    binary = confined(project, directory / ("stef.exe" if goos == "windows" else "stef"))
    data = source.read_bytes()
    if not data:
        raise InstallError("The stef binary is empty.")
    if binary.is_file() and hashlib.sha256(binary.read_bytes()).digest() == hashlib.sha256(data).digest():
        return binary
    descriptor, name = tempfile.mkstemp(prefix=".stef-install-", dir=directory)
    try:
        with os.fdopen(descriptor, "wb") as file:
            file.write(data)
            file.flush()
            os.fsync(file.fileno())
        if os.name != "nt":
            Path(name).chmod(0o755)
        try:
            os.replace(name, binary)
        except PermissionError as error:
            raise InstallError("Close running stef processes before updating its binary, then reconnect its MCP. Your existing stef was preserved.") from error
    finally:
        Path(name).unlink(missing_ok=True)
    return binary


def source_tools():
    paths = {}
    for tool in ["git", "go", "node", "npm.cmd" if os.name == "nt" else "npm"]:
        paths[tool] = shutil.which(tool)
        if not paths[tool]:
            raise InstallError("No matching portable release is available. Source builds need Git, Go 1.25+, Node 22.12+ and npm. Missing: " + tool + ".")
    version = subprocess.run([paths["go"], "version"], check=True, capture_output=True, text=True).stdout
    match = re.search(r"go(\d+)\.(\d+)", version)
    if not match or tuple(map(int, match.groups())) < (1, 25):
        raise InstallError("Source builds need Go 1.25+.")
    version = subprocess.run([paths["node"], "--version"], check=True, capture_output=True, text=True).stdout.strip()
    match = re.fullmatch(r"v?(\d+)\.(\d+)\.\d+(?:-[\w.-]+)?", version)
    if not match or tuple(map(int, match.groups())) < (22, 12):
        raise InstallError("Source builds need Node 22.12+ and npm.")
    return paths["git"]


def source_binary(project, source_dir, goos, view):
    if source_dir:
        source = Path(source_dir).expanduser()
        if not source.is_absolute():
            raise InstallError("--source-dir must be an absolute stef source path.")
        source = source.resolve()
    else:
        source = confined(project, project / ".stef/runtime/stef")
    git = source_tools()
    if source.exists():
        if not (source / ".git").exists() or not (source / "daemon/go.mod").is_file() or not (source / "scripts/build.py").is_file():
            raise InstallError("Existing source directory is not a current stef checkout; choose --source-dir.")
        origin = subprocess.run([git, "-C", str(source), "remote", "get-url", "origin"], check=True, capture_output=True, text=True).stdout.strip()
        if origin not in [REPOSITORY, REPOSITORY + ".git", "git@github.com:ivandosevv/stef.git"]:
            raise InstallError("The source checkout belongs to another repository; your files were preserved.")
    else:
        source.parent.mkdir(parents=True, exist_ok=True)
        subprocess.run([git, "clone", "--depth", "1", "--branch", "master", REPOSITORY + ".git", str(source)],
                       check=True, stdout=sys.stderr, stderr=sys.stderr)
    arguments = [sys.executable, str(source / "scripts/build.py"), "--source-dir", str(source)]
    if view == "native" and goos == "darwin":
        arguments.append("--native")
    subprocess.run(arguments, check=True, cwd=source, stdout=sys.stderr, stderr=sys.stderr)
    compiled = source / "bin" / ("stef.exe" if goos == "windows" else "stef")
    binary = install_binary(project, compiled, goos)
    install_license(project, source / "LICENSE")
    return binary


def install_license(project, source):
    destination = confined(project, project / ".stef/runtime/LICENSE")
    data = source.read_bytes()
    descriptor, temporary = tempfile.mkstemp(prefix=".license-", dir=destination.parent)
    try:
        with os.fdopen(descriptor, "wb") as file:
            file.write(data)
        os.replace(temporary, destination)
    finally:
        Path(temporary).unlink(missing_ok=True)


def install(args):
    project = project_root(args.repo)
    confined(project, project / ".stef/runtime")
    goos, goarch = target()
    payload = None if args.source else release_payload(args.release or "latest", goos, goarch)
    if payload:
        archive, extension, version = payload
        directory = confined(project, project / ".stef/runtime")
        directory.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(prefix=".stef-release-", dir=directory) as temporary:
            extracted = Path(temporary)
            extract_archive(archive, extension, extracted)
            compiled = extracted / ("stef.exe" if goos == "windows" else "stef")
            if not compiled.is_file() or not (extracted / "LICENSE").is_file():
                raise InstallError("The release is missing its stef binary or license.")
            binary = install_binary(project, compiled, goos)
            install_license(project, extracted / "LICENSE")
        mode = "release"
    else:
        if not args.source:
            print("No matching portable release yet; building stef from source in its own runtime folder.", file=sys.stderr)
        binary = source_binary(project, args.source_dir, goos, args.view)
        mode, version = "source", None
    receipt = {"installed": True, "install_mode": mode, "executable": str(binary), "repo": str(project)}
    if version:
        receipt["version"] = version
    if args.build_only:
        receipt["enabled"] = False
        return receipt
    arguments = [str(binary), "--enable", "--client", args.client, "--view", args.view, "--repo", str(project)]
    if args.daemon:
        arguments.extend(["--daemon", args.daemon])
    result = subprocess.run(arguments, check=True, cwd=project, capture_output=True, text=True, encoding="utf-8")
    if result.stderr:
        print(result.stderr, file=sys.stderr, end="")
    receipt.update(json.loads(result.stdout))
    return receipt


def main(argv=None):
    if sys.version_info < (3, 9):
        print("stef install needs Python 3.9+; Windows release setup also works with bootstrap.ps1 alone.", file=sys.stderr)
        return 1
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True, help="Absolute path to your current project; never the stef checkout by accident")
    parser.add_argument("--client", required=True, help="The agent doing the work, such as codex, claude or cursor; generic exports MCP settings")
    parser.add_argument("--view", choices=["ide", "native"], default="ide")
    parser.add_argument("--source-dir", help="Reuse this absolute stef source checkout without pulling or resetting it")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--source", action="store_true", help="Build from source with Git, Go and Node; no make or Bash needed")
    mode.add_argument("--release", help="Use a published version, such as 0.3.0; default latest")
    parser.add_argument("--build-only", action="store_true", help="Install the binary without configuring or starting the agent companion")
    parser.add_argument("--daemon", help="Optional explicit local daemon endpoint; default selects a project port")
    args = parser.parse_args(argv)
    if not args.client.strip():
        parser.error("--client must identify the actual agent, or generic")
    try:
        print(json.dumps(install(args)))
        return 0
    except subprocess.CalledProcessError as error:
        if error.stderr:
            print(error.stderr, file=sys.stderr, end="")
        print("stef install: a build or setup command failed; your project selection was preserved.", file=sys.stderr)
        return 1
    except (InstallError, OSError, ValueError, tarfile.TarError, zipfile.BadZipFile) as error:
        print("stef install: " + str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
