"""Build stef with Python, npm and Go on macOS, Windows or Linux."""
import argparse
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tempfile


class BuildError(Exception):
    pass


def platform_target(system=None, machine=None):
    system = platform.system() if system is None else system
    machine = platform.machine() if machine is None else machine
    goos = {"Darwin": "darwin", "Windows": "windows", "Linux": "linux"}.get(system)
    goarch = {"x86_64": "amd64", "AMD64": "amd64", "amd64": "amd64",
              "arm64": "arm64", "aarch64": "arm64", "ARM64": "arm64"}.get(machine)
    if not goos or not goarch:
        raise BuildError("Supported source targets are macOS, Windows and Linux on x64 or ARM64.")
    return goos, goarch


def executable(name):
    candidate = shutil.which(name)
    if not candidate:
        raise BuildError("Source builds need Go 1.25+, Node 22.12+ and npm. Missing: " + name + ".")
    return candidate


def prerequisites():
    go, node = executable("go"), executable("node")
    npm = executable("npm.cmd" if os.name == "nt" else "npm")
    version = subprocess.run([go, "version"], check=True, capture_output=True, text=True).stdout
    match = re.search(r"go(\d+)\.(\d+)", version)
    if not match or tuple(map(int, match.groups())) < (1, 25):
        raise BuildError("Source builds need Go 1.25+.")
    version = subprocess.run([node, "--version"], check=True, capture_output=True, text=True).stdout.strip()
    match = re.fullmatch(r"v?(\d+)\.(\d+)\.\d+(?:-[\w.-]+)?", version)
    if not match or tuple(map(int, match.groups())) < (22, 12):
        raise BuildError("Source builds need Node 22.12+ and npm.")
    # Node's Windows installation includes this JS entry beside npm.cmd. Run it
    # through Node when present so paths are never parsed by a command shell.
    cli = Path(npm).parent / "node_modules/npm/bin/npm-cli.js"
    npm_command = [node, str(cli)] if os.name == "nt" and cli.is_file() else [npm]
    return go, npm_command


def build(source, *, native=False, skip_install=False, goos=None, goarch=None, output=None):
    source = Path(source).resolve(strict=True)
    if not (source / "daemon/go.mod").is_file() or not (source / "ui/package-lock.json").is_file():
        raise BuildError("Choose a stef source checkout with --source-dir.")
    host_os, host_arch = platform_target()
    goos, goarch = goos or host_os, goarch or host_arch
    if native and (host_os != "darwin" or goos != "darwin"):
        raise BuildError("Native source builds currently use macOS WebKit; use the portable default on Windows or Linux.")
    binary = Path(output) if output else source / "bin" / ("stef.exe" if goos == "windows" else "stef")
    if not binary.is_absolute():
        raise BuildError("--output must be an absolute path inside the source checkout.")
    binary = binary.resolve()
    if source not in binary.parents:
        raise BuildError("--output must stay inside the source checkout.")
    go, npm = prerequisites()
    env = os.environ.copy()
    env.update(CGO_ENABLED="1" if native else "0", GOOS=goos, GOARCH=goarch)
    def run(arguments, cwd):
        subprocess.run(arguments, cwd=cwd, env=env, check=True, stdout=sys.stderr, stderr=sys.stderr)
    if not skip_install:
        run([*npm, "ci"], source / "ui")
        run([go, "mod", "download"], source / "daemon")
    run([*npm, "run", "build"], source / "ui")
    run([sys.executable, str(source / "scripts/embed-ui.py")], source)
    binary.parent.mkdir(parents=True, exist_ok=True)
    # A failed compile cannot replace the last working binary.
    descriptor, temporary = tempfile.mkstemp(prefix=".stef-build-", suffix=".exe" if goos == "windows" else "", dir=binary.parent)
    os.close(descriptor)
    try:
        run([go, "build", "-o", temporary, "."], source / "daemon")
        if os.name != "nt":
            Path(temporary).chmod(0o755)
        os.replace(temporary, binary)
    finally:
        Path(temporary).unlink(missing_ok=True)
    return binary


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-dir", type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--native", action="store_true", help="Build the optional macOS native window; portable is the default")
    parser.add_argument("--skip-install", action="store_true", help="Use already installed npm and Go dependencies")
    parser.add_argument("--goos", choices=["darwin", "linux", "windows"])
    parser.add_argument("--goarch", choices=["amd64", "arm64"])
    parser.add_argument("--output", type=Path)
    parser.add_argument("--check", action="store_true", help="Check source-build prerequisites without building")
    args = parser.parse_args(argv)
    try:
        if args.check:
            prerequisites()
            print(json.dumps({"source_tools_ready": True}))
            return 0
        binary = build(args.source_dir, native=args.native, skip_install=args.skip_install,
                       goos=args.goos, goarch=args.goarch, output=args.output)
        print(json.dumps({"executable": str(binary), "portable": not args.native}))
        return 0
    except (BuildError, OSError, subprocess.CalledProcessError) as error:
        print("stef build: " + str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
