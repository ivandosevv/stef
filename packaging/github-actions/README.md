# GitHub Actions

.github/workflows/check.yml builds and tests the portable runtime on macOS, Windows, and Linux,
including MCP startup and exact saved diffs without build tools on PATH. Linux
also runs the browser suite. .github/workflows/release.yml verifies those hosts before producing
six x64/ARM64 packages and publishing a new release.

Confirm a check run on each OS before publishing through the
release workflow. Existing release assets are preserved. Local source setup uses
install.py; Python build and verification commands work without make or Bash.
