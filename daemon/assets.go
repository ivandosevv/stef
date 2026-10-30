package main

import "embed"

// Built by make build. The placeholder keeps headless development builds usable.
//
//go:embed assets
var bundledUI embed.FS
