#!/usr/bin/env bash
set -euo pipefail
# Source-build compatibility entry. install.py also supports verified releases.
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)
if command -v python3 >/dev/null 2>&1; then
  exec python3 "$script_dir/../install.py" --source "$@"
fi
printf '%s\n' 'Source setup needs Python 3.9+, Git, Go 1.25+, Node 22.12+ and npm. No make is required.' >&2
exit 1
