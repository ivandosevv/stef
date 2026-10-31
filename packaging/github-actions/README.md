# GitHub Actions

.github/workflows/check.yml runs lint, Go race checks, UI unit tests, and seven
focused browser tests on pull requests. Portable checks cover macOS, Windows,
and Linux, including installers, MCP startup, saved diffs, and dependency scans.
Master pushes and manual runs also run the full browser suite.

Master requires all five check jobs and a pull request, including for admins.
Use squash merge after resolving review conversations. Branches are deleted
after merging. See [CONTRIBUTING.md](../../CONTRIBUTING.md).

.github/workflows/release.yml accepts only source already merged into master.
It verifies those hosts before producing six x64/ARM64 packages and publishing
a new release.

Confirm a check run on each OS before publishing through the
release workflow. Existing release assets are preserved. Local source setup uses
install.py; Python build and verification commands work without make or Bash.
