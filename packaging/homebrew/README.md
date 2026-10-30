# Brew delivery preparation

stef packages macOS, Windows, and Linux on x64 and ARM64. Portable installation
uses install.py, so Brew is optional. make release packages the current build in
artifacts/releases with SHA256 checksums; it does not publish anything.

For an optional macOS ARM64 recipe, run scripts/package-release.py with
--brew-formula --url-prefix <published-base-url>. Verify its URL and checksum
before publishing a tap. The generated recipe stays separate from the portable
Windows/Linux/macOS archives.

A public brew install stef command is not available yet. Release archives bundle
the renderer and need no Node runtime or IDE plugin.
