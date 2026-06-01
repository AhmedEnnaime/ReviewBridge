# ReviewBridge — CLAUDE.md

## Project Overview

ReviewBridge is a local CLI daemon written in Go that bridges GitHub PR and GitLab MR review comments directly into Claude Code sessions. It polls GitHub/GitLab APIs for new review comments, triages them using the Claude API, shows a Bubble Tea TUI approval dialog, and invokes `claude --resume <session-id>` to fix approved comments in the right session — without the developer having to copy-paste anything.

**Repository:** https://github.com/ahmedennaime/reviewbridge  
**Language:** Go 1.26.3  
**Status:** In development (MVP)

---

## Architecture

```
cmd/reviewbridge/main.go     — CLI entry point (cobra)
internal/
  config/     — config file loading (~/.reviewbridge/config.yaml)
  daemon/     — main orchestrator + session watcher (fsnotify)
  db/         — SQLite via modernc/sqlite, migrations in db/migrations/
  dialog/     — Bubble Tea TUI for triage approval
  notify/     — desktop + terminal notifications (beeep)
  platforms/  — GitHub and GitLab API clients behind a shared interface
  poller/     — polling loop + startup catch-up
  queue/      — comment state machine
  runner/     — invokes claude --resume as a subprocess
  session/    — watches ~/.claude/projects/ for new Claude Code sessions
  triage/     — calls Claude Messages API for comment triage
tests/
  integration/  — needs Docker WireMock running (make test-integration)
  e2e/          — full daemon flow tests (make test-e2e)
  mocks/        — WireMock JSON stub files for GitHub and GitLab APIs
skills/         — Claude Code slash command definitions
```

---

## Module Pattern

Every internal package follows the same structure:
- One file per responsibility (no god files)
- Exported types and interfaces at the top of each file
- Unexported implementation below
- Tests in `*_test.go` beside the file they test

Example for `platforms/`:
```
platforms/
  interface.go       — Platform interface + shared PullRequest/Comment structs
  github/client.go   — GitHub implementation
  gitlab/client.go   — GitLab implementation
```

---

## Key Interfaces

```go
// platforms/interface.go
type Platform interface {
    ListOpenPullRequests(repo string) ([]PullRequest, error)
    GetPullRequest(repo string, prID int) (*PullRequest, error)
    ListCommentsSince(repo string, prID int, since time.Time) ([]Comment, error)
    GetDiff(repo string, prID int) (string, error)
}
```

All internal packages that need platform access accept a `Platform` interface, never a concrete struct. This is what makes unit tests possible without real API credentials.

---

## Database

SQLite file at `~/.reviewbridge/reviewbridge.db`. Three tables: `sessions`, `pull_requests`, `comments`. Migrations are numbered SQL files in `internal/db/migrations/`. Never modify a migration that has already been run — always create a new one.

The `modernc/sqlite` driver is used (pure Go, no CGO, no system dependency).

---

## Comment Lifecycle (State Machine)

```
fetched → triaged → queued → in_progress → done
                 ↘ skip
         (if session busy) → parked → queued
```

The `queue` package owns all state transitions. No other package should write comment states directly to the DB — always go through `queue`.

---

## Session Mapping

Claude Code sessions are stored as JSONL files in `~/.claude/projects/<repo-path>/`. The daemon watches this directory via `fsnotify`. When a new session appears, it immediately reads the current git branch of that repo and records: `session_id → branch_name`. GitHub/GitLab PRs/MRs have a `source_branch` field. The join is: `session.branch_name == pr.branch_name`.

---

## Claude Runner

```go
// runner/claude.go
Run(sessionID string, prompt string) (*RunResult, error)
```

Invokes `claude --resume <sessionID> -p "<prompt>"` as a subprocess. Always uses `--resume` with the mapped session ID — never starts a new session. Timeout is 10 minutes (configurable). Output is scanned for git commit hashes to populate `RunResult.CommitHash`.

---

## Development Commands

```bash
# Build
make build

# Run all unit tests (no Docker)
make test

# Integration tests (requires Docker)
make test-integration

# End-to-end tests (requires Docker + claude CLI)
make test-e2e

# Coverage
make test-cover

# Lint
golangci-lint run ./...

# Run daemon locally
./bin/reviewbridge start

# Run daemon in Docker
make dev
```

---

## Environment Variables

See `.env.example` for all variables. Copy to `.env` for local development.

Key vars:
- `ANTHROPIC_API_KEY` — required, used by triage engine
- `GITHUB_TOKEN` — required for GitHub support
- `GITLAB_TOKEN` — optional, for GitLab support
- `REVIEWBRIDGE_GITHUB_BASE_URL` — override for tests (points at WireMock)
- `REVIEWBRIDGE_GITLAB_BASE_URL` — override for tests (points at WireMock)

---

## Testing Strategy

- **Unit tests** — no external dependencies, use `httptest.NewServer` for HTTP mocking
- **Integration tests** — use Docker WireMock containers (`make test-integration`), no real credentials
- **E2E tests** — full daemon flow, use WireMock + real `claude` CLI subprocess

Never use real GitHub/GitLab credentials in CI. All CI tests use WireMock.

---

## Do's

- Use the `Platform` interface everywhere — never import `github` or `gitlab` packages directly from outside `platforms/`
- Run state transitions through the `queue` package only
- Add a new SQL migration file for every schema change — never edit existing ones
- All user-facing strings go through the notification or dialog packages — no raw `fmt.Println` in business logic
- Test files go beside the file they test (`foo_test.go` next to `foo.go`)

## Don'ts

- Don't make HTTP calls to GitHub/GitLab directly from `daemon`, `triage`, or `runner` — go through the `Platform` interface
- Don't write comment states directly to the DB — always use `queue` package functions
- Don't start new Claude Code sessions — always use `--resume` with a mapped session ID
- Don't auto-resolve PR/MR review threads — that is always the reviewer's job
- Don't commit `.env` files or real API tokens
