# Changelog

## v1.0.0 — 2026-06-05

First public release.

### Features

- **Daemon** — background process that polls GitHub PRs and GitLab MRs every 60 seconds
- **Triage engine** — Claude API evaluates every review comment and labels it `fix`, `your-call`, or `skip`
- **Queue file** — approved comments written to `~/.reviewbridge/queue/<branch>.json`; survives daemon restarts
- **`/check-reviews` skill** — Claude Code slash command that reads the queue, applies `fix` comments immediately, pauses on `your-call` items, and leaves changes unstaged for you to review and commit
- **Desktop notifications** — notified the moment new comments are triaged (macOS and Linux)
- **`reviewbridge init`** — guided setup that validates Anthropic and GitHub/GitLab tokens before writing config
- **`reviewbridge start / stop / status`** — daemon lifecycle management
- **`reviewbridge install-skill`** — installs the `/check-reviews` slash command into Claude Code
- **Catch-up on restart** — daemon recovers all missed comments when it starts after being offline
- **GitHub and GitLab support** — including self-hosted GitLab instances
- **Zero CGO** — pure Go binary, no system libraries required
- **Platform support** — macOS (amd64, arm64) and Linux (amd64, arm64)
