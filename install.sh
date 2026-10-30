#!/bin/sh
# Install a verified release in the user's current project, without build tools.
set -eu
umask 077

fail() { printf 'stef install: %s\n' "$*" >&2; exit 1; }
usage() {
  cat <<'HELP'
Install and enable stef beside the agent in your current project.
Usage: install.sh --repo /absolute/project --client NAME
                  [--view ide|native] [--release latest|0.3.0]
                  [--build-only] [--daemon http://127.0.0.1:PORT]
Needs curl, tar, and shasum or sha256sum. No Python or build tools.
Defaults to the IDE view; installs only inside <project>/.stef/runtime.
HELP
}

_stef_repo=
_stef_client=
_stef_view=ide
_stef_release=latest
_stef_daemon=
_stef_build_only=0
_stef_stage=
while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo|--client|--view|--release|--daemon)
      [ "$#" -ge 2 ] || fail "Missing value for $1."
      case "$1" in
        --repo) _stef_repo=$2 ;;
        --client) _stef_client=$2 ;;
        --view) _stef_view=$2 ;;
        --release) _stef_release=$2 ;;
        --daemon) _stef_daemon=$2 ;;
      esac
      shift 2 ;;
    --build-only) _stef_build_only=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) fail "Unknown option: $1. Use --help." ;;
  esac
done
case "$_stef_repo" in /*) ;; *) fail 'Choose an existing absolute --repo for your current project.' ;; esac
[ -d "$_stef_repo" ] || fail 'The selected project folder does not exist.'
_stef_repo=$(CDPATH= cd "$_stef_repo" && pwd -P)
_stef_home=
if [ -n "${HOME:-}" ] && [ -d "$HOME" ]; then _stef_home=$(CDPATH= cd "$HOME" && pwd -P); fi
[ "$_stef_repo" != / ] && [ "$_stef_repo" != "$_stef_home" ] || fail 'Choose your project folder, not your home folder or a filesystem root.'
[ -n "$_stef_client" ] || fail 'Choose the agent doing the work with --client; use generic for other MCP clients.'
case "$_stef_view" in ide|native) ;; *) fail '--view must be ide or native.' ;; esac
for _stef_tool in curl tar awk cat chmod cmp cut date mkdir mktemp mv rm uname wc; do
  command -v "$_stef_tool" >/dev/null 2>&1 || fail "Missing standard tool: $_stef_tool."
done
if command -v sha256sum >/dev/null 2>&1; then _stef_hash=sha256sum
elif command -v shasum >/dev/null 2>&1; then _stef_hash=shasum
else fail 'SHA256 verification needs shasum or sha256sum.'; fi
case "$(uname -s)" in Darwin) _stef_os=darwin ;; Linux) _stef_os=linux ;; *) fail 'Use the PowerShell installer on Windows; this installer supports macOS and Linux.' ;; esac
case "$(uname -m)" in x86_64|amd64) _stef_arch=amd64 ;; arm64|aarch64) _stef_arch=arm64 ;; *) fail 'Release binaries support x64 and ARM64.' ;; esac

assert_directory() {
  _stef_path="$_stef_repo/$1"
  if [ -e "$_stef_path" ] || [ -L "$_stef_path" ]; then
    [ -d "$_stef_path" ] || fail 'Keep .stef/runtime as a directory inside your project.'
    _stef_resolved=$(CDPATH= cd "$_stef_path" && pwd -P)
    case "$_stef_resolved" in "$_stef_repo"/*) ;; *) fail 'The stef runtime escapes your project through a directory link.' ;; esac
  fi
}
for _stef_relative in .stef .stef/runtime .stef/runtime/bin .stef/runtime/skills .stef/runtime/skills/stef; do assert_directory "$_stef_relative"; done
_stef_runtime="$_stef_repo/.stef/runtime"
_stef_binary="$_stef_runtime/bin/stef"
for _stef_path in "$_stef_binary" "$_stef_runtime/LICENSE" "$_stef_runtime/skills/stef/SKILL.md"; do
  [ ! -L "$_stef_path" ] || fail 'The existing stef runtime files cannot be links.'
  if [ -e "$_stef_path" ]; then [ -f "$_stef_path" ] || fail 'An existing stef runtime file is not a regular file.'; fi
done
mkdir -p "$_stef_runtime/bin"
assert_directory .stef/runtime/bin
_stef_stage=$(mktemp -d "$_stef_runtime/bin/.stef-install-XXXXXX")
cleanup() { if [ -n "$_stef_stage" ]; then rm -rf "$_stef_stage"; fi; }
trap cleanup 0
trap 'exit 130' 2
trap 'exit 143' 15

validate_url() {
  case "$1" in https://*) ;; *) fail 'GitHub redirected outside HTTPS release hosts.' ;; esac
  _stef_authority=${1#https://}
  _stef_authority=${_stef_authority%%/*}
  case "$_stef_authority" in
    github.com) case "$1" in https://github.com/ivandosevv/stef/releases/*) ;; *) fail 'The release URL does not belong to stef.' ;; esac ;;
    release-assets.githubusercontent.com|objects.githubusercontent.com|github-releases.githubusercontent.com) ;;
    *) fail 'GitHub redirected outside its release hosts.' ;;
  esac
}

# Follow redirects only after checking the next host, and bound time and bytes.
download() {
  _stef_url=$1
  _stef_destination=$2
  _stef_limit=$3
  _stef_deadline=$(( $(date +%s) + 60 ))
  _stef_redirects=0
  while :; do
    validate_url "$_stef_url"
    _stef_remaining=$(( _stef_deadline - $(date +%s) ))
    [ "$_stef_remaining" -gt 0 ] || fail 'GitHub download timed out; retry the install.'
    _stef_result=$(
      ulimit -f $(( (_stef_limit + 511) / 512 )) 2>/dev/null || exit 1
      curl --fail --silent --show-error --proto '=https' --connect-timeout 10 \
        --max-time "$_stef_remaining" --max-filesize "$_stef_limit" \
        --dump-header "$_stef_stage/headers" --output "$_stef_destination" \
        --write-out '%{http_code} %{url_effective}' "$_stef_url"
    ) || fail 'Could not download a published stef release. Retry, or use the source installer.'
    _stef_status=${_stef_result%% *}
    _stef_effective_url=${_stef_result#* }
    validate_url "$_stef_effective_url"
    [ "$(wc -c < "$_stef_destination")" -le "$_stef_limit" ] || fail 'The release download is too large.'
    case "$_stef_status" in
      200) return ;;
      301|302|303|307|308)
        _stef_redirects=$(( _stef_redirects + 1 ))
        [ "$_stef_redirects" -le 5 ] || fail 'Too many GitHub release redirects.'
        _stef_location=$(awk 'tolower($0) ~ /^location:/ { sub(/^[^:]*:[ \t]*/, ""); sub(/\r$/, ""); location=$0; count++ } END { if (count==1) print location; else exit 1 }' "$_stef_stage/headers") || fail 'GitHub returned an invalid redirect.'
        case "$_stef_location" in
          https://*) _stef_url=$_stef_location ;;
          /*) _stef_url="https://$_stef_authority$_stef_location" ;;
          *) fail 'GitHub returned an unsupported release redirect.' ;;
        esac ;;
      *) fail 'GitHub returned an unexpected release response.' ;;
    esac
  done
}

if [ "$_stef_release" = latest ]; then
  download https://github.com/ivandosevv/stef/releases/latest "$_stef_stage/release-page" 2097152
  case "$_stef_effective_url" in
    https://github.com/ivandosevv/stef/releases/tag/v*) _stef_release=${_stef_effective_url#https://github.com/ivandosevv/stef/releases/tag/v} ;;
    *) fail 'GitHub did not return a published stef release tag.' ;;
  esac
else _stef_release=${_stef_release#v}; fi
printf '%s\n' "$_stef_release" | awk '/^[0-9]+[.][0-9]+[.][0-9]+(-[A-Za-z0-9.-]+)?$/ { good=1 } END { exit !(good && NR==1) }' || fail 'Use latest or a release version such as 0.3.0.'
_stef_archive_name="stef-$_stef_release-$_stef_os-$_stef_arch.tar.gz"
_stef_base="https://github.com/ivandosevv/stef/releases/download/v$_stef_release"
download "$_stef_base/$_stef_archive_name.sha256" "$_stef_stage/checksum" 4096
_stef_expected=$(awk -v name="$_stef_archive_name" 'NF==2 && length($1)==64 && $1 !~ /[^0-9A-Fa-f]/ && ($2==name || $2=="*" name) { hash=tolower($1); count++ } END { if (count==1 && NR==1) print hash; else exit 1 }' "$_stef_stage/checksum") || fail 'The release SHA256 file is invalid.'
download "$_stef_base/$_stef_archive_name" "$_stef_stage/archive" 104857600
if [ "$_stef_hash" = sha256sum ]; then _stef_actual=$(sha256sum "$_stef_stage/archive" | awk '{print $1}')
else _stef_actual=$(shasum -a 256 "$_stef_stage/archive" | awk '{print $1}'); fi
[ "$_stef_actual" = "$_stef_expected" ] || fail 'Release checksum mismatch. Your existing stef was preserved.'
# Bound decompression work and listing output as well as downloaded bytes.
bounded_tar() {
  _stef_tar_limit=$1
  shift
  (
    ulimit -f $(( (_stef_tar_limit + 511) / 512 )) 2>/dev/null || exit 1
    ulimit -t 30 2>/dev/null || exit 1
    tar "$@"
  )
}
bounded_tar 2097152 -tzf "$_stef_stage/archive" > "$_stef_stage/members" || fail 'The release archive is invalid or exceeds inspection limits.'
[ "$(wc -c < "$_stef_stage/members")" -le 2097152 ] || fail 'The release archive has too many members.'

extract_member() {
  _stef_member=$1
  _stef_member_destination=$2
  _stef_required=$3
  _stef_member_limit=$4
  _stef_count=$(awk -v name="$_stef_member" '$0==name { count++ } END { print count+0 }' "$_stef_stage/members")
  if [ "$_stef_count" = 0 ] && [ "$_stef_required" = 0 ]; then return; fi
  [ "$_stef_count" = 1 ] || fail 'The release needs one regular file for each selected member.'
  bounded_tar 65536 -tvzf "$_stef_stage/archive" "$_stef_member" > "$_stef_stage/member-type" || fail 'The release member cannot be inspected.'
  [ "$(wc -c < "$_stef_stage/member-type")" -le 65536 ] || fail 'The release member listing is too large.'
  [ "$(wc -l < "$_stef_stage/member-type")" -eq 1 ] && [ "$(cut -c 1 "$_stef_stage/member-type")" = - ] || fail 'Release links and non-regular selected members are not supported.'
  bounded_tar "$_stef_member_limit" -xOzf "$_stef_stage/archive" "$_stef_member" > "$_stef_member_destination" || fail 'The release member could not be extracted safely.'
  [ "$(wc -c < "$_stef_member_destination")" -le "$_stef_member_limit" ] || fail 'The extracted release file is too large.'
  [ -s "$_stef_member_destination" ] || fail 'The selected release file is empty.'
}
extract_member stef "$_stef_stage/stef" 1 104857600
extract_member LICENSE "$_stef_stage/LICENSE" 0 1048576
extract_member skills/stef/SKILL.md "$_stef_stage/SKILL.md" 0 1048576
chmod 755 "$_stef_stage/stef"
if [ -f "$_stef_stage/LICENSE" ]; then mv -f "$_stef_stage/LICENSE" "$_stef_runtime/LICENSE"; fi
if [ -f "$_stef_stage/SKILL.md" ]; then
  mkdir -p "$_stef_runtime/skills/stef"
  assert_directory .stef/runtime/skills/stef
  mv -f "$_stef_stage/SKILL.md" "$_stef_runtime/skills/stef/SKILL.md"
fi
if [ -f "$_stef_binary" ] && cmp -s "$_stef_stage/stef" "$_stef_binary"; then
  [ -x "$_stef_binary" ] || chmod 755 "$_stef_binary"
else mv -f "$_stef_stage/stef" "$_stef_binary"; fi

json_string() {
  printf '%s' "$1" | awk 'BEGIN { printf "\""; for (i=1;i<32;i++) controls[sprintf("%c",i)]=i } { if (NR>1) printf "\\n"; for (i=1;i<=length($0);i++) { c=substr($0,i,1); if (c=="\\") printf "\\\\"; else if (c=="\"") printf "\\\""; else if (c in controls) printf "\\u%04x",controls[c]; else printf "%s",c } } END { printf "\"" }'
}
if [ "$_stef_build_only" = 1 ]; then
  printf '{"installed":true,"install_mode":"release","enabled":false,"executable":'
  json_string "$_stef_binary"
  printf ',"repo":'; json_string "$_stef_repo"
  printf ',"version":'; json_string "$_stef_release"
  printf '}\n'
  exit 0
fi
cleanup
_stef_stage=
if [ -n "$_stef_daemon" ]; then
  exec "$_stef_binary" --enable --client "$_stef_client" --view "$_stef_view" --repo "$_stef_repo" --daemon "$_stef_daemon"
else exec "$_stef_binary" --enable --client "$_stef_client" --view "$_stef_view" --repo "$_stef_repo"; fi
