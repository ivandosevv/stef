#!/usr/bin/env bash
set -euo pipefail
usage() {
  cat <<'HELP'
Build stef from source and configure one project's coding agent.
Usage: bootstrap.sh --repo /absolute/project --client NAME [--view ide|native]
                    [--source-dir /absolute/stef-source] [--build-only]
Requires Git, Go 1.25+, Node 22.12+, npm, Python 3, and make.
macOS native builds also require Xcode Command Line Tools.
Defaults: --view ide; source inside <repo>/.stef/runtime/stef.
Existing checkouts are reused without pulling or overwriting edits.
No toolchains, IDE extensions, global client profiles, or PATH entries are installed.
HELP
}
repo=''
client=''
view='ide'
source_dir=''
build_only=0
while (($#)); do
  case "$1" in
    --repo|--client|--view|--source-dir)
      if (($# < 2)); then printf 'Missing value for %s\n' "$1" >&2; exit 2; fi
      case "$1" in
        --repo) repo=$2 ;;
        --client) client=$2 ;;
        --view) view=$2 ;;
        --source-dir) source_dir=$2 ;;
      esac
      shift 2 ;;
    --build-only) build_only=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done
if [[ -z "$repo" || -z "$client" || "$repo" != /* || ! -d "$repo" ]]; then
  printf 'Choose an existing absolute --repo and its actual --client.\n' >&2
  exit 2
fi
if [[ "$view" != ide && "$view" != native ]]; then
  printf '%s\n' '--view must be ide or native.' >&2; exit 2
fi
repo=$(cd "$repo" && pwd -P)
if [[ -z "$source_dir" ]]; then
  source_dir="$repo/.stef/runtime/stef"
  python3 - "$repo" "$source_dir" <<'PYROOT'
from pathlib import Path
import sys
repo=Path(sys.argv[1]).resolve()
source=Path(sys.argv[2]).resolve()
if source != repo and repo not in source.parents:
    raise SystemExit('Default source directory escapes the project through a symlink; choose an explicit --source-dir.')
PYROOT
fi
if [[ "$source_dir" != /* ]]; then printf '%s\n' '--source-dir must be absolute.' >&2; exit 2; fi
for required in git go node npm python3 make; do
  if ! command -v "$required" >/dev/null 2>&1; then
    printf 'Missing prerequisite: %s. Install the documented build tools first.\n' "$required" >&2
    exit 1
  fi
done
node -e 'const [major, minor] = process.versions.node.split(".").map(Number); if (major < 22 || (major === 22 && minor < 12)) { console.error("Node 22.12+ is required."); process.exit(1) }'
python3 - "$(go version)" <<'PY'
import re, sys
match=re.search(r'go(\d+)\.(\d+)',sys.argv[1])
if not match or tuple(map(int,match.groups())) < (1,25):
    raise SystemExit('Go 1.25+ is required.')
PY
if [[ -e "$source_dir" ]]; then
  if [[ ! -d "$source_dir/.git" || ! -f "$source_dir/Makefile" || ! -f "$source_dir/daemon/go.mod" ]]; then
    printf 'Existing source directory is not a stef checkout: %s\n' "$source_dir" >&2
    exit 1
  fi
  origin=$(git -C "$source_dir" remote get-url origin)
  case "$origin" in
    https://github.com/ivandosevv/stef|https://github.com/ivandosevv/stef.git|git@github.com:ivandosevv/stef.git) ;;
    *) printf 'Source checkout has a different origin; choose the intended stef source directory.\n' >&2; exit 1 ;;
  esac
else
  mkdir -p "$(dirname "$source_dir")"
  git clone --depth 1 --branch master https://github.com/ivandosevv/stef.git "$source_dir"
fi
make -C "$source_dir" install
make -C "$source_dir" build
if ((build_only)); then
  printf 'Built stef: %s/bin/stef\n' "$source_dir"
  exit 0
fi
exec "$source_dir/bin/stef" --enable --client "$client" --view "$view" --repo "$repo"
