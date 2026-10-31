"""Run portable Go, TypeScript and Svelte checks without modifying source."""
import os
from pathlib import Path
import subprocess
import sys

from build import BuildError, prerequisites


def main():
    root = Path(__file__).resolve().parent.parent
    go, npm = prerequisites()
    tracked = subprocess.run(
        ["git", "ls-files", "-z", "--", "*.go"],
        cwd=root, check=True, capture_output=True, text=True,
    ).stdout
    files = [path for path in tracked.split("\0") if path]
    if files:
        result = subprocess.run(
            ["gofmt", "-l", *files],
            cwd=root, check=True, capture_output=True, text=True,
        )
        if result.stdout.strip():
            print("Run gofmt on these files:\n" + result.stdout, file=sys.stderr)
            return 1
    env = os.environ.copy()
    env.setdefault("CGO_ENABLED", "0")
    subprocess.run([go, "vet", "./..."], cwd=root / "daemon", env=env, check=True)
    subprocess.run([*npm, "run", "lint"], cwd=root / "ui", check=True)
    subprocess.run([*npm, "run", "check"], cwd=root / "ui", check=True)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (BuildError, OSError, subprocess.CalledProcessError) as error:
        print("stef lint: " + str(error), file=sys.stderr)
        raise SystemExit(1)
