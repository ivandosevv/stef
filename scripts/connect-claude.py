"""Opt-in personal Claude registration, bound to one explicitly chosen project.

Uses Claude's native user-scope MCP command. Project setup remains separate,
and this never makes the home directory the observer root.
"""
import argparse
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile


SOURCE = Path(__file__).resolve().parent.parent


def confined(root, path):
    resolved = path.resolve()
    if not resolved.is_relative_to(root):
        raise ValueError(f"Personal skill runtime escapes the selected project: {path}")
    return path


def connect(repo, config_dir, endpoint=None):
    repo = repo.resolve(strict=True)
    if not repo.is_dir() or repo == Path.home().resolve():
        raise ValueError("Choose a project directory; the home directory is not an observer root")
    executable = SOURCE / "bin/stef"
    if not executable.is_file():
        raise ValueError("Build stef with make build before connecting Claude")
    claude = shutil.which("claude")
    if not claude:
        raise ValueError("Claude Code is not on PATH")
    config_dir = config_dir.expanduser().absolute()
    profile = (Path.home() / ".claude.json" if config_dir == Path.home() / ".claude"
               else config_dir / ".claude.json")
    user_skill = config_dir / "skills/stef"
    runtime = confined(repo, repo / ".stef/claude-user/stef")
    skill_file = confined(repo, runtime / "SKILL.md")
    if user_skill.exists() or user_skill.is_symlink():
        if not user_skill.is_symlink() or user_skill.resolve() != runtime.resolve():
            raise ValueError("An existing personal stef skill belongs to another setup; it was preserved")
    existing = None
    if profile.exists():
        document = json.loads(profile.read_text())
        if not isinstance(document, dict) or not isinstance(document.get("mcpServers", {}), dict):
            raise ValueError("The existing Claude user profile has an invalid MCP mapping; it was preserved")
        existing = document.get("mcpServers", {}).get("stef")
    setup_cmd = [str(executable), "--enable", "--client", "claude",
                 "--view", "ide", "--repo", str(repo)]
    if endpoint:
        setup_cmd.extend(["--daemon", endpoint])
    # Inspect existing user configuration before performing any project setup.
    if existing is not None:
        expected_prefix = ["--mcp", "--repo", str(repo), "--daemon"]
        args = existing.get("args", []) if isinstance(existing, dict) else []
        if (not isinstance(existing, dict) or existing.get("command") != str(executable)
                or len(args) != 5 or args[:4] != expected_prefix
                or existing.get("url") is not None
                or existing.get("type", "stdio") != "stdio"
                or (endpoint and args[4] != endpoint)):
            raise ValueError("The existing user MCP named stef has a different target; it was preserved")
        if endpoint is None:
            setup_cmd.extend(["--daemon", args[4]])
    result = subprocess.run(setup_cmd, cwd=SOURCE, capture_output=True, text=True, check=True)
    setup = json.loads(result.stdout)
    endpoint = setup["endpoint"]
    desired = ["--mcp", "--repo", str(repo), "--daemon", endpoint]
    env = os.environ.copy()
    env["CLAUDE_CONFIG_DIR"] = str(config_dir)
    if existing is None:
        subprocess.run([claude, "mcp", "add", "--scope", "user", "--transport",
                        "stdio", "stef", "--", str(executable), *desired],
                       cwd=SOURCE, env=env, check=True, capture_output=True, text=True)
    runtime.mkdir(parents=True, exist_ok=True)
    template = (SOURCE / "integrations/claude/personal/SKILL.md").read_text()
    replacements = {"repo": str(repo), "endpoint": endpoint,
                    "connect_command": shlex.join(["python3", str(SOURCE / "scripts/connect-claude.py"),
                                                    "--repo", str(repo), "--daemon", endpoint])}
    for key, value in replacements.items():
        template = template.replace("{{" + key + "}}", value)
    with tempfile.NamedTemporaryFile(mode="w", dir=runtime, delete=False) as output:
        output.write(template)
        temporary = Path(output.name)
    temporary.replace(skill_file)
    user_skill.parent.mkdir(parents=True, exist_ok=True)
    if not user_skill.is_symlink():
        user_skill.symlink_to(runtime, target_is_directory=True)
    return {"client": "claude", "scope": "user", "repo": str(repo),
            "endpoint": endpoint, "view_url": setup["view_url"],
            "skill_path": str(user_skill / "SKILL.md"),
            "next_step": "Restart Claude once; /stef and the stef MCP then load from your personal settings."}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=SOURCE, help="Explicit personal project to observe")
    parser.add_argument("--daemon", help="Existing loopback stef endpoint")
    parser.add_argument("--config-dir", type=Path,
                        default=Path(os.environ.get("CLAUDE_CONFIG_DIR", Path.home() / ".claude")),
                        help="Claude config directory; defaults to the normal personal profile")
    options = parser.parse_args()
    try:
        print(json.dumps(connect(options.repo, options.config_dir, options.daemon)))
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(str(error)) from error


if __name__ == "__main__":
    main()
