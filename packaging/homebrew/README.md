# Brew delivery preparation

The current runtime is a single macOS ARM64 executable. make release creates an
unsigned archive in artifacts/, a SHA256 file, and Formula/stef.rb with the exact
checksum and a local file URL. Nothing is uploaded or globally installed.

For a release owner, scripts/package-release.py --url-prefix <published-base-url>
can regenerate the recipe to point at that archive's eventual public location.
A Homebrew tap and release publication are separate future steps. The homepage
in the recipe is the intended repository location, not a claim it already exists.

A public brew install stef command is not available yet. Current working setup
uses the local build followed by one command, or the MCP configuration that starts
the native companion. The archive contains no Node runtime or IDE plugin.
