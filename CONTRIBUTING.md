# Contributing

Create a branch, make your change, and open a pull request into `master`.
Explain what changed and how you checked it. Small, focused PRs are easier to review.

For local development, use Go 1.25+, Node 24 (or 22.13+), and Python 3.9+:

```sh
make install
make lint
make test
```

Windows users can run `python scripts/build.py`, `python scripts/lint.py`,
`go test ./...` from `daemon`, and `npm test` from `ui` without make.
For workflow or browser changes, also run `npm run test:e2e` from `ui` after building.

PRs need passing lint, core tests, and macOS, Windows, and Linux checks.
Resolve review conversations before merging. Maintainers squash merge PRs;
direct pushes, force pushes, and deleting `master` are blocked.

Tests and local previews make no AI calls. Keep credentials, personal MCP
settings, runtime history, and generated files out of commits.
