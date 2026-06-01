# ReviewBridge — Plan

> A local CLI daemon that monitors GitHub PRs and GitLab MRs for review comments from any reviewer, triages them with Claude, and routes approved fixes directly into the correct Claude Code session — without breaking your current flow.

---

## Problem

Developers using Claude Code hit a constant copy-paste loop:
1. Open a PR or MR
2. Reviewer (human, Copilot, CodeRabbit, anyone) leaves comments
3. Developer manually reads the comments on GitHub/GitLab
4. Copies them into Claude Code
5. Claude Code fixes them
6. Developer goes back to GitHub/GitLab to check

ReviewBridge eliminates steps 3 and 4 entirely. Comments flow automatically from GitHub/GitLab into the right Claude Code session, with a triage step so the developer stays in control.

---

## What It Is

A **local CLI daemon** — a background process that runs on the developer's machine.

- Not a web app (no server, no hosting, no accounts)
- Not a plugin (not tied to any editor)
- Not a cloud CI/CD runner (not GitHub Actions)
- Runs locally, code stays on your machine
- Works regardless of what editor or IDE you use

```bash
reviewbridge start      # starts the daemon in the background
reviewbridge status     # shows what's being monitored
reviewbridge queue      # see all pending comments
reviewbridge stop       # stops the daemon
```

---

## Core Principles

- **Platform agnostic** — GitHub and GitLab, with the same experience on both
- **Reviewer agnostic** — works for comments from humans, Copilot, CodeRabbit, or any bot
- **Developer stays in control** — nothing happens without an explicit approval step
- **Context is never lost** — always resumes the existing Claude Code session, never starts a new one
- **Non-intrusive** — if you're busy, comments queue silently until you're ready
- **Thread resolution is the reviewer's job** — ReviewBridge never marks threads as resolved

---

## Architecture Overview

```
[GitHub API / GitLab API]
        │
        │  polling every 60s (or webhooks if configured)
        ▼
[ReviewBridge daemon — Go, runs locally]
        │
        ├── New comments detected on tracked PR/MR
        │
        ├── Silent triage call → Claude API (fast, cheap)
        │   Reads: diff + comments + project CLAUDE.md
        │   Returns verdict per comment: fix / your-call / skip
        │
        ├── Desktop or terminal notification
        │
        ├── Bubble Tea triage dialog
        │   User reviews verdicts, adjusts, confirms
        │
        └── claude --resume <session-id> -p "[approved comments]"
                │
                Runs headlessly — separate from active session
                Full conversation history preserved
                │
                ▼
           Fixes applied, commit created
                │
                ▼
           Notification: "Fixed 2 comments — commit a3f91bc"
```

---

## How Session Mapping Works

Every PR or MR has a source branch. Every Claude Code session runs inside a repository on a specific branch. ReviewBridge uses the **branch as the common key** to link PRs/MRs to sessions.

### Mapping Chain

```
Claude Code session abc123
        │  daemon records branch at session creation
        ▼
Branch: feature/issue-a
        │  GitHub/GitLab API: PR has source_branch field
        ▼
PR #12 (GitHub) or MR #7 (GitLab)
        │  API polling
        ▼
Review comments → routed back to session abc123
```

### How the Link Is Established

**Path A — Daemon was running when session opened (primary)**

The daemon watches `~/.claude/projects/<repo>/` for new JSONL session files. The moment one appears, it reads the current git branch:

```bash
git -C <repo_path> branch --show-current
# feature/issue-a
```

Records: `session abc123 → branch feature/issue-a`. When the API later reports `PR #12 source_branch: feature/issue-a`, the link is automatic.

**Path B — Daemon was offline when session opened (fallback)**

On startup, the daemon finds untracked sessions. It reads:
- Working directory from JSONL tool call paths
- Session creation timestamp from the JSONL first entry
- Git log around that timestamp to infer the branch

If confident → records silently. If ambiguous → asks once:

```
Found untracked session from 3h ago in /repos/myapp
Looks like branch: feature/issue-a — correct? [Y/n]
```

**Path C — Manual override (always available)**

```bash
reviewbridge link --session abc123 --branch feature/issue-a
reviewbridge link --session abc123 --pr 12
```

---

## Handling Edge Cases

### Multiple PRs or MRs on the Same Branch

Technically possible (same source branch targeting different base branches). Not a problem — all those PRs/MRs are the same code, same work. Comments from all of them route to the same session. In the triage dialog they are grouped by PR/MR number:

```
Branch: feature/issue-a — 3 new comments

  PR #12 (→ main)
    ✅ line 42 — missing null check

  PR #13 (→ develop)
    ✅ line 87 — array could be empty
    ⚠️  line 103 — your call
```

### Daemon Was Offline, Comments Arrived While Computer Was Off

Each PR/MR has a `last_checked_at` timestamp stored in the local database. On startup, the daemon immediately queries the API for all comments newer than that timestamp on every tracked open PR/MR. Comments that arrived offline are fetched, triaged, and queued exactly as if they had arrived in real time. Nothing is lost.

```
Daemon starts up
→ Checks last_checked_at for all open PRs/MRs
→ Fetches all missed comments in one pass
→ Triages and queues them
→ Notification: "3 comments arrived while you were away on PR #47"
```

### Comments Arrive for Issue A While Working on Issue C

The most important case. The daemon tracks multiple sessions independently. It detects that the active session is for a different branch and handles it without interruption:

```
User actively working in session xyz789 (branch: feature/issue-c)
Comments arrive for PR #12 (branch: feature/issue-a → session abc123)

Daemon detects: different sessions — no conflict
Runs silently: claude --resume abc123 -p "[approved comments]"

This subprocess is fully independent of session xyz789
User keeps working on issue C without switching context
Notification appears when done: "PR #12 comments fixed — commit d4e91f2"
```

The session mismatch case becomes an advantage: comments get fixed in the background while the developer keeps working on something else.

### Comments Arrive While the Matching Session Is Actively Open in Terminal

If `session abc123` is currently live and interactive (user is typing in it), the daemon cannot inject a message mid-conversation. It queues the comments silently and shows a subtle notification:

```
Comments on PR #12 queued — session for feature/issue-a is active
Will process when session is free, or run /check-reviews to pull them now
```

The user can either wait for their current task to finish (daemon processes automatically) or type `/check-reviews` — a custom Claude Code slash command installed by ReviewBridge — to pull the queued comments directly into the current session themselves.

---

## Triage Step — Detail

Before showing any dialog, ReviewBridge makes a fast call to the Claude API (not Claude Code — just the API directly, a small cheap call) with:
- The PR/MR diff
- All new comments
- The project's CLAUDE.md if present

Claude returns a verdict for each comment:

| Verdict | Meaning |
|---|---|
| `fix` | Clear issue, matches your conventions, safe to fix |
| `your-call` | Valid point but requires a decision — architectural, ambiguous, or conflicts with existing patterns |
| `skip` | Style nitpick, already handled, or irrelevant to current work |

The developer sees this in the triage dialog and can override any verdict before approving:

```
4 new comments on PR #47 — feature/issue-a

  ✅ fix      line 42  (@john)       Missing null check on user input
  ✅ fix      line 87  (Copilot)     Array access without length check
  ⚠️  your-call line 103 (CodeRabbit) Suggests extracting to separate service
  ❌ skip     line 201 (@john)       Prefers different variable naming style

[Approve selected]  [Edit]  [Later]
```

Pressing "Later" puts everything in the queue. Nothing is lost.

---

## Comment State Machine

```
fetched
  → triaged (fix / your-call / skip)
       │
  user approves
       │
  queued
       │
  in_progress (claude --resume running)
       │
  done (commit created)

  (if session busy or user hits Later)
  queued → parked → queued (when session frees)
```

---

## Local Database Schema (SQLite)

```sql
sessions (
  session_id       TEXT PRIMARY KEY,   -- Claude Code JSONL filename
  repo_path        TEXT,               -- /Users/you/repos/myapp
  branch_name      TEXT,               -- feature/issue-a
  last_active_at   DATETIME,
  status           TEXT                -- active | idle | closed
)

pull_requests (
  pr_id            TEXT PRIMARY KEY,   -- "github:owner/repo:12" or "gitlab:group/repo:7"
  platform         TEXT,               -- github | gitlab
  repo             TEXT,
  branch_name      TEXT,
  session_id       TEXT REFERENCES sessions,
  last_checked_at  DATETIME,
  status           TEXT                -- open | closed | merged
)

comments (
  comment_id       TEXT PRIMARY KEY,
  pr_id            TEXT REFERENCES pull_requests,
  author           TEXT,
  body             TEXT,
  file_path        TEXT,
  line_number      INTEGER,
  created_at       DATETIME,
  fetched_at       DATETIME,
  triage_verdict   TEXT,               -- fix | your-call | skip | pending
  state            TEXT                -- fetched | triaged | queued | parked | in_progress | done
)
```

---

## Technical Architecture

```
reviewbridge/
├── cmd/
│   └── reviewbridge/
│       └── main.go              # Entry point, CLI commands setup
│
├── internal/
│   ├── daemon/
│   │   ├── daemon.go            # Main loop, orchestrates everything
│   │   └── watcher.go           # Watches ~/.claude/projects/ for new sessions
│   │
│   ├── platforms/
│   │   ├── interface.go         # Platform interface (common contract)
│   │   ├── github/
│   │   │   ├── client.go        # GitHub REST API client
│   │   │   └── webhook.go       # Optional webhook listener
│   │   └── gitlab/
│   │       ├── client.go        # GitLab REST API client
│   │       └── webhook.go       # Optional webhook listener
│   │
│   ├── session/
│   │   ├── tracker.go           # Watches for new Claude Code sessions
│   │   ├── reader.go            # Reads JSONL session files
│   │   └── registry.go          # Maintains session → branch mapping
│   │
│   ├── triage/
│   │   ├── triage.go            # Calls Claude API for triage
│   │   └── prompt.go            # Builds triage prompt from diff + comments
│   │
│   ├── dialog/
│   │   └── triage_ui.go         # Bubble Tea TUI for triage approval dialog
│   │
│   ├── runner/
│   │   └── claude.go            # Invokes claude --resume as subprocess
│   │
│   ├── queue/
│   │   └── queue.go             # Comment queue management
│   │
│   ├── db/
│   │   ├── db.go                # SQLite connection + migrations
│   │   └── queries.go           # All database operations
│   │
│   └── notify/
│       └── notify.go            # Desktop + terminal notifications
│
├── skills/
│   └── check-reviews.md         # Claude Code /check-reviews slash command
│
├── config/
│   └── config.go                # Config file parsing (~/.reviewbridge/config.yaml)
│
├── go.mod
├── go.sum
└── README.md
```

---

## Tech Stack

| Concern | Choice | Why |
|---|---|---|
| Language | Go | Single binary, low memory (~8MB idle), great for daemons, no runtime dependency |
| TUI | Bubble Tea | Best Go TUI library, clean interactive dialogs |
| SQLite | modernc/sqlite | Embedded, no DB server, perfect for local state — pure Go, no CGO |
| GitHub API | `google/go-github` | Well-maintained official-quality client |
| GitLab API | `xanzy/go-gitlab` | Most complete GitLab Go client |
| Claude triage | HTTP (stdlib) | Direct API call, no SDK needed — just POST to messages endpoint |
| Session watching | `fsnotify` | Cross-platform file system watcher for `~/.claude/projects/` |
| Notifications | `gen2brain/beeep` | Cross-platform desktop notifications (Mac/Linux/Windows) |
| Distribution | Homebrew + GitHub Releases | Single binary, `brew install reviewbridge` |
| Dev environment | Docker + Docker Compose | Consistent Go toolchain across machines, no "works on my machine" |
| Test infrastructure | Docker Compose + WireMock | Mock GitHub and GitLab APIs for integration and E2E tests |

---

## Docker & Infrastructure

### Why Docker Here

ReviewBridge's runtime database is **SQLite** — an embedded file-based database that ships inside the Go binary. It requires no server, no Docker container, and no setup. The DB file lives at `~/.reviewbridge/reviewbridge.db` on the developer's machine and is created automatically on first run.

Docker is used for two other purposes:

**1. Development Environment (`docker-compose.dev.yml`)**

Ensures every contributor uses the same Go version and tools, with no local Go installation required:

```yaml
# docker-compose.dev.yml
services:
  app:
    build:
      context: .
      dockerfile: Dockerfile.dev
    volumes:
      - .:/workspace
      - ~/.reviewbridge:/root/.reviewbridge
      - ~/.claude:/root/.claude
    environment:
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - GITHUB_TOKEN=${GITHUB_TOKEN}
    command: go run ./cmd/reviewbridge start
```

```dockerfile
# Dockerfile.dev
FROM golang:1.23-alpine
WORKDIR /workspace
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
```

Run the daemon in Docker during development:
```bash
docker compose -f docker-compose.dev.yml up
```

**2. Test Infrastructure (`docker-compose.test.yml`)**

Runs mock GitHub and GitLab API servers for integration and E2E tests. Uses **WireMock** — a battle-tested HTTP mock server with a Docker image. Test code points at `http://localhost:8080` instead of `api.github.com`, enabling realistic API tests without real credentials and without hitting rate limits.

```yaml
# docker-compose.test.yml
services:
  mock-github:
    image: wiremock/wiremock:latest
    ports:
      - "8080:8080"
    volumes:
      - ./tests/mocks/github:/home/wiremock/mappings
    command: --verbose --local-response-templating

  mock-gitlab:
    image: wiremock/wiremock:latest
    ports:
      - "8081:8080"
    volumes:
      - ./tests/mocks/gitlab:/home/wiremock/mappings
    command: --verbose --local-response-templating
```

Mock response stubs live in `tests/mocks/github/` and `tests/mocks/gitlab/` as JSON files:

```json
// tests/mocks/github/list-prs.json
{
  "request": { "method": "GET", "url": "/repos/owner/repo/pulls?state=open" },
  "response": {
    "status": 200,
    "headers": { "Content-Type": "application/json" },
    "bodyFileName": "list-prs-response.json"
  }
}
```

Start test infrastructure before running integration or E2E tests:
```bash
docker compose -f docker-compose.test.yml up -d
REVIEWBRIDGE_MOCK_GITHUB_URL=http://localhost:8080 go test ./tests/integration/...
docker compose -f docker-compose.test.yml down
```

**3. Release Build (`Dockerfile.build`)**

Produces the final cross-platform binaries in a clean environment — ensures the release binary was not built with any local machine quirks:

```dockerfile
# Dockerfile.build
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o reviewbridge ./cmd/reviewbridge
```

### Updated Folder Structure (with Docker files)

```
reviewbridge/
├── cmd/reviewbridge/main.go
├── internal/                        # all internal packages (unchanged)
├── tests/
│   ├── integration/
│   ├── e2e/
│   └── mocks/
│       ├── github/                  # WireMock stub files for GitHub API
│       │   ├── list-prs.json
│       │   ├── list-comments.json
│       │   └── get-diff.json
│       └── gitlab/                  # WireMock stub files for GitLab API
│           ├── list-mrs.json
│           ├── list-notes.json
│           └── get-diff.json
├── Dockerfile.dev                   # Development container
├── Dockerfile.build                 # Release build container
├── docker-compose.dev.yml           # Dev environment
├── docker-compose.test.yml          # Test infrastructure (WireMock servers)
├── .env.example                     # Template: ANTHROPIC_API_KEY, GITHUB_TOKEN, etc.
├── go.mod
├── go.sum
└── README.md
```

---

## Configuration File

Lives at `~/.reviewbridge/config.yaml`:

```yaml
anthropic_api_key: sk-ant-...

platforms:
  github:
    token: ghp_...
    polling_interval: 60s       # or use webhooks
  gitlab:
    token: glpat-...
    url: https://gitlab.com     # or self-hosted URL
    polling_interval: 60s

claude_code:
  sessions_path: ~/.claude/projects   # default, usually no need to change

triage:
  auto_skip_style_comments: true      # auto-skip comments flagged as style-only
  min_confidence: medium              # only show fix verdict if Claude is confident

notifications:
  desktop: true
  terminal: true
```

---

## Setup Flow (Developer Experience)

```bash
# Install
brew install reviewbridge

# First-time setup (guided)
reviewbridge init

? Anthropic API key: sk-ant-...
? GitHub token (for reading PR comments): ghp_...
? GitLab token (optional, press Enter to skip): 
✔ Config saved to ~/.reviewbridge/config.yaml

# Start the daemon
reviewbridge start
✔ Daemon started — watching for new Claude Code sessions

# See what's being tracked
reviewbridge status

Active sessions:
  abc123  /repos/myapp        feature/issue-a  → PR #12 (GitHub)
  xyz789  /repos/myapp        feature/issue-c  → PR #15 (GitHub)

# View pending queue
reviewbridge queue

  [queued]  PR #12  line 42  (@john)  Missing null check
  [parked]  PR #12  line 87  (Copilot)  Array access
```

---

## The /check-reviews Slash Command

ReviewBridge installs a custom Claude Code slash command at `~/.claude/commands/check-reviews.md`:

```markdown
Check the ReviewBridge queue for pending review comments on the current branch.
Read the file at ~/.reviewbridge/queue/<current-branch>.json if it exists.
If there are pending comments, list them and ask which ones to fix.
```

The user can type `/check-reviews` at any point in any Claude Code session to manually pull in queued comments for that branch. This covers the case where the session is actively open and the user wants to handle comments themselves without waiting.

---

## MVP Milestones

| Phase | Scope | Goal |
|---|---|---|
| **v0.1** | GitHub polling + session watcher + branch mapping + `claude --resume` invocation | End-to-end flow working on GitHub |
| **v0.2** | Bubble Tea triage dialog + SQLite state + offline catch-up | Production-quality UX |
| **v0.3** | GitLab support + webhook option + multi-session routing | Full platform coverage |
| **v0.4** | `/check-reviews` slash command + `reviewbridge queue` command + config file | Developer experience complete |
| **v1.0** | Polished setup flow, docs, Homebrew formula, Product Hunt launch | Public release |

---

## What This Is Not

- Not a code review tool — it doesn't generate new comments, it routes existing ones
- Not a cloud service — everything runs on your machine
- Not a replacement for reading reviews — the triage dialog keeps you informed of every decision
- Not an auto-resolver — marking threads as resolved is always the reviewer's job

---

## Why This Will Get Used

1. **Zero friction** — `brew install`, `reviewbridge start`, done
2. **Works with any reviewer** — human teammates, Copilot, CodeRabbit, all treated the same
3. **Context preserved** — always resumes the right session, never starts fresh
4. **Non-intrusive** — you can ignore notifications and nothing breaks, queue is always there
5. **Background fixing** — comments for one PR get fixed while you work on another
6. **No lock-in** — if you stop using it, nothing in your workflow breaks
