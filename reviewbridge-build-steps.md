# ReviewBridge — Step-by-Step Build Plan

> Implementation roadmap from zero to v1.0. Each step has a clear goal, what to build, and test cases to verify before moving on.

---

## Ground Rules

- Never move to the next step until all test cases for the current step pass
- Each step produces something runnable or testable — no long stretches of untestable code
- Unit tests live next to the code they test (`foo_test.go` beside `foo.go`)
- Integration tests are in `tests/integration/` and require real credentials (skipped in CI unless explicitly enabled)
- End-to-end tests are in `tests/e2e/` and test the full daemon flow

---

## Step 1 — Project Scaffold + Docker Infrastructure

**Goal:** A compiling Go project with the right folder structure, a working CLI entry point, a config file that loads correctly, and a Docker environment that any contributor can use immediately.

### What to Build

**Go project:**
- Initialize Go module: `go mod init github.com/yourname/reviewbridge`
- Create the full folder structure from the architecture plan
- `cmd/reviewbridge/main.go` — entry point with `cobra` CLI framework
- Register placeholder commands: `start`, `stop`, `status`, `queue`, `init`, `link`
- `internal/config/config.go` — reads `~/.reviewbridge/config.yaml` using `viper`
- `~/.reviewbridge/config.yaml` template with all fields documented

**Docker files:**
- `Dockerfile.dev` — Go 1.23 Alpine image for local development, mounts the repo as a volume so code changes reflect immediately without rebuilding
- `Dockerfile.build` — clean multi-stage build image for producing release binaries (`CGO_ENABLED=0`)
- `docker-compose.dev.yml` — runs the daemon inside Docker, mounts `~/.reviewbridge` and `~/.claude` so the container shares the host's session state and config
- `docker-compose.test.yml` — spins up two WireMock containers: one mocking GitHub API on port `8080`, one mocking GitLab API on port `8081`
- `tests/mocks/github/` and `tests/mocks/gitlab/` — empty directories with a `README.md` explaining stub file format (stubs are added in Step 4)
- `.env.example` — documents all required environment variables (`ANTHROPIC_API_KEY`, `GITHUB_TOKEN`, `GITLAB_TOKEN`)

### Dependencies to Add
```
cobra         — CLI commands and flags
viper         — config file parsing
```

### Docker Files Content

```dockerfile
# Dockerfile.dev
FROM golang:1.23-alpine
RUN apk add --no-cache git
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
```

```dockerfile
# Dockerfile.build
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS=linux TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o reviewbridge ./cmd/reviewbridge

FROM scratch
COPY --from=builder /app/reviewbridge /reviewbridge
ENTRYPOINT ["/reviewbridge"]
```

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
    working_dir: /workspace
    environment:
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - GITLAB_TOKEN=${GITLAB_TOKEN}
    command: go run ./cmd/reviewbridge start
```

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

### Test Cases

**Unit — config loader**
- `TestConfigLoadsFromFile` — create a temp config YAML, load it, assert all fields parsed correctly
- `TestConfigMissingFile` — when config file doesn't exist, returns a clear error (not a panic)
- `TestConfigMissingAPIKey` — when `anthropic_api_key` is empty, `config.Validate()` returns error
- `TestConfigDefaultPollingInterval` — when `polling_interval` is not set, defaults to 60s
- `TestConfigGitLabURLDefaults` — when `gitlab.url` is not set, defaults to `https://gitlab.com`

**Docker checks**
- `docker compose -f docker-compose.dev.yml build` completes without error
- `docker compose -f docker-compose.test.yml up -d` starts both WireMock containers
- `curl http://localhost:8080/__admin/health` returns `{ "status": "ok" }` (GitHub mock healthy)
- `curl http://localhost:8081/__admin/health` returns `{ "status": "ok" }` (GitLab mock healthy)
- `docker compose -f docker-compose.test.yml down` stops both cleanly

**Manual check**
- Run `reviewbridge --help` and see all commands listed
- Run `reviewbridge start` and see "not implemented yet" message (no crash)
- Run `docker compose -f docker-compose.dev.yml run app reviewbridge --help` — same output from inside Docker

---

## Step 2 — Database Layer

**Goal:** A working SQLite database that initializes itself on first run and exposes clean query functions for every table.

### What to Build
- `internal/db/db.go` — opens SQLite at `~/.reviewbridge/reviewbridge.db`, runs migrations on startup
- `internal/db/migrations/` — SQL migration files (numbered: `001_init.sql`, etc.)
- `001_init.sql` — creates `sessions`, `pull_requests`, `comments` tables exactly as defined in the plan
- `internal/db/queries.go` — one function per database operation:
  - `SaveSession`, `GetSession`, `GetSessionByBranch`, `ListActiveSessions`
  - `SavePullRequest`, `GetPullRequest`, `ListOpenPullRequests`, `UpdateLastChecked`
  - `SaveComment`, `GetComment`, `UpdateCommentState`, `ListCommentsByPR`, `ListQueuedComments`

### Dependencies to Add
```
modernc.org/sqlite   — pure Go SQLite driver (no CGO needed)
```

### Test Cases

**Unit — migrations**
- `TestMigrationsRunOnEmptyDB` — new empty DB runs all migrations without error
- `TestMigrationsIdempotent` — running migrations twice does not error or duplicate tables
- `TestSchemaHasAllTables` — after migration, `sessions`, `pull_requests`, `comments` all exist

**Unit — session queries**
- `TestSaveAndGetSession` — save a session, retrieve by ID, assert all fields match
- `TestGetSessionByBranch` — save two sessions with different branches, retrieve by branch name returns the right one
- `TestGetSessionByBranch_NotFound` — querying a branch with no session returns nil, no error
- `TestListActiveSessions` — save 3 sessions (2 active, 1 closed), list returns only 2

**Unit — pull request queries**
- `TestSaveAndGetPullRequest` — save a PR, retrieve by ID, assert all fields
- `TestUpdateLastChecked` — save a PR, update `last_checked_at`, retrieve and assert updated time
- `TestListOpenPullRequests` — save 4 PRs (3 open, 1 merged), list returns only 3
- `TestLinkPRToSession` — save PR and session separately, link them, retrieve PR and assert `session_id` is set

**Unit — comment queries**
- `TestSaveAndGetComment` — save a comment, retrieve by ID, assert all fields
- `TestUpdateCommentState` — save comment as `fetched`, update to `triaged`, assert state changed
- `TestListCommentsByPR` — save 5 comments across 2 PRs, list by PR returns only that PR's comments
- `TestListQueuedComments` — save 6 comments with mixed states, list queued returns only `queued` ones

---

## Step 3 — Session Tracker

**Goal:** The daemon detects when a new Claude Code session opens, reads its metadata, captures the current git branch, and saves to the database.

### What to Build
- `internal/session/watcher.go` — uses `fsnotify` to watch `~/.claude/projects/` for new JSONL files
- `internal/session/reader.go` — reads a Claude Code JSONL session file, extracts:
  - Session ID (from filename)
  - Repo path (from file paths in tool calls)
  - Session creation timestamp (from first entry)
- `internal/session/branch.go` — runs `git -C <repo_path> branch --show-current` and returns the branch name
- `internal/session/registry.go` — orchestrates: on new session detected → read metadata → get branch → save to DB

### Dependencies to Add
```
fsnotify   — cross-platform file system watcher
```

### Test Cases

**Unit — JSONL reader**
- `TestReaderExtractsRepoPath` — given a JSONL with file tool calls at `/repos/myapp/main.go`, extracts `/repos/myapp`
- `TestReaderExtractsSessionID` — given filename `abc123.jsonl`, extracts `abc123`
- `TestReaderExtractsTimestamp` — first entry in JSONL has a timestamp, reader returns it
- `TestReaderEmptyFile` — empty JSONL returns error, no panic
- `TestReaderNoToolCalls` — JSONL with only text messages (no file calls) returns empty repo path

**Unit — branch detector**
- `TestGetBranchFromValidRepo` — point at a real local git repo, returns current branch name
- `TestGetBranchDetachedHEAD` — repo in detached HEAD state returns empty string and specific error
- `TestGetBranchNotARepo` — non-git directory returns clear error, no panic
- `TestGetBranchEmptyRepo` — git repo with no commits returns clear error

**Unit — registry**
- `TestRegistryCreatesSessionOnNewFile` — drop a JSONL file into watched dir, registry saves session to DB
- `TestRegistryIgnoresNonJSONLFiles` — drop a `.txt` file into watched dir, nothing is saved
- `TestRegistryIgnoresDuplicates` — same JSONL modified twice, session is not duplicated in DB

**Integration — watcher (requires real filesystem)**
- `TestWatcherDetectsNewSession` — create temp dir, start watcher, write a JSONL file, assert callback fires within 2s

---

## Step 4 — Platform Clients

**Goal:** Working GitHub and GitLab API clients that fetch PR/MR details and comments, behind a shared interface so the rest of the code doesn't care which platform it's talking to.

### What to Build
- `internal/platforms/interface.go` — defines the `Platform` interface:
  ```go
  type Platform interface {
      ListOpenPullRequests(repo string) ([]PullRequest, error)
      GetPullRequest(repo string, prID int) (*PullRequest, error)
      ListCommentsSince(repo string, prID int, since time.Time) ([]Comment, error)
      GetDiff(repo string, prID int) (string, error)
  }
  ```
- `internal/platforms/github/client.go` — implements `Platform` using GitHub REST API
- `internal/platforms/gitlab/client.go` — implements `Platform` using GitLab REST API
- Shared `PullRequest` and `Comment` structs in `interface.go` that both clients map to

### Dependencies to Add
```
google/go-github   — GitHub API client
xanzy/go-gitlab    — GitLab API client
```

### Test Cases

**Unit — GitHub client (uses mock HTTP server)**
- `TestGitHubListOpenPRs` — mock server returns PR list JSON, client parses and returns correct `[]PullRequest`
- `TestGitHubListCommentsSince` — mock returns 5 comments, 2 before the `since` time; client returns only 3
- `TestGitHubGetDiff` — mock returns a diff string, client returns it unchanged
- `TestGitHubUnauthorized` — mock returns 401, client returns a clear auth error
- `TestGitHubRateLimited` — mock returns 429, client returns a rate-limit error (not a generic error)
- `TestGitHubRepoNotFound` — mock returns 404, client returns a not-found error

**Unit — GitLab client (uses mock HTTP server)**
- `TestGitLabListOpenMRs` — same as GitHub equivalent but for MR JSON format
- `TestGitLabListCommentsSince` — same logic, GitLab uses "notes" instead of "comments"
- `TestGitLabGetDiff` — GitLab diff format is different from GitHub, assert correct mapping
- `TestGitLabUnauthorized` — 401 returns clear auth error
- `TestGitLabSelfHostedURL` — client initialized with `https://gitlab.mycompany.com` sends requests there, not to `gitlab.com`

**Integration — via Docker WireMock (no real credentials needed)**

Add WireMock stub files in `tests/mocks/github/` and `tests/mocks/gitlab/` to simulate the API responses, then run:

```bash
# Start mock servers
docker compose -f docker-compose.test.yml up -d

# Run integration tests pointed at mock servers
REVIEWBRIDGE_GITHUB_BASE_URL=http://localhost:8080 \
REVIEWBRIDGE_GITLAB_BASE_URL=http://localhost:8081 \
go test ./tests/integration/...

# Tear down
docker compose -f docker-compose.test.yml down
```

- `TestGitHubIntegration_ListPRs` — WireMock returns stub PR list, client parses correctly end-to-end
- `TestGitHubIntegration_RateLimitRetry` — WireMock returns 429 then 200, client retries and succeeds
- `TestGitHubIntegration_CommentsWithPagination` — WireMock returns paginated comments (2 pages), client fetches all
- `TestGitLabIntegration_ListMRs` — WireMock returns stub MR list in GitLab format
- `TestGitLabIntegration_NotesWithPagination` — WireMock returns paginated notes, client fetches all
- `TestGitLabIntegration_SelfHosted` — client pointed at `http://localhost:8081` (simulating self-hosted), requests go there not to `gitlab.com`

**Integration — real credentials (optional, skip in CI unless explicitly enabled)**
```bash
REVIEWBRIDGE_INTEGRATION=true \
GITHUB_TOKEN=ghp_... \
go test ./tests/integration/ -run TestRealAPI
```
- `TestGitHubIntegration_RealAPI` — hit real GitHub API with a test repo, assert PRs returned
- `TestGitLabIntegration_RealAPI` — hit real GitLab API with a test project, assert MRs returned

---

## Step 5 — PR/MR Poller

**Goal:** A polling loop that runs on a configurable interval, checks all tracked PRs/MRs for new comments, saves them to the database, and advances their state.

### What to Build
- `internal/poller/poller.go` — main polling loop:
  - Loads all open PRs/MRs from DB
  - For each: calls `platform.ListCommentsSince(last_checked_at)`
  - Saves new comments to DB with state `fetched`
  - Updates `last_checked_at` to now
- `internal/poller/startup.go` — catch-up on startup: runs the same fetch for all PRs/MRs regardless of polling interval, so offline comments are recovered immediately
- PR discovery: when a session is linked to a branch, the poller queries the platform for open PRs/MRs on that branch and registers them

### Test Cases

**Unit — polling loop**
- `TestPollerFetchesNewComments` — mock platform returns 3 comments, poller saves all 3 to DB with state `fetched`
- `TestPollerUpdatesLastChecked` — after a successful poll, `last_checked_at` for the PR is updated
- `TestPollerSkipsClosedPRs` — PR with status `merged` or `closed` in DB is not polled
- `TestPollerDeduplicatesComments` — platform returns comment already in DB, poller does not duplicate it
- `TestPollerHandlesPlatformError` — platform call returns error, poller logs it and continues with next PR (does not crash)
- `TestPollerRespectsPollInterval` — mock clock, assert poller does not call platform before interval elapses

**Unit — startup catch-up**
- `TestStartupCatchUpFetchesAllMissedComments` — PR has `last_checked_at` 6 hours ago, platform returns 4 comments from that period, all 4 are saved
- `TestStartupCatchUpRunsBeforeFirstInterval` — assert catch-up runs immediately at daemon start, not after waiting for first interval

**Unit — PR discovery**
- `TestDiscoversPRForNewBranch` — new session on `feature/issue-a`, platform has open PR for that branch, PR is registered in DB and linked to session
- `TestNoPRForBranch` — new session on branch with no open PR, nothing is registered, no error
- `TestMultiplePRsForBranch` — branch has 2 PRs to different base branches, both are registered

---

## Step 6 — Triage Engine

**Goal:** Given a PR diff and a list of comments, call the Claude API and get back a verdict for each comment (fix / your-call / skip) with a short reason.

### What to Build
- `internal/triage/prompt.go` — builds the triage prompt:
  - System prompt explaining the triage task
  - PR diff (truncated if too long, with a summary note)
  - Each comment with author, file, line, body
  - CLAUDE.md content if found in the repo
  - Asks Claude to return structured JSON verdicts
- `internal/triage/triage.go` — calls Claude Messages API, parses response JSON into `[]TriageResult`
- `TriageResult` struct: `CommentID`, `Verdict` (fix/your-call/skip), `Reason` (one sentence)
- Diff truncation logic: if diff exceeds token budget, include changed files list + first N lines of diff with a note

### Dependencies to Add
```
net/http standard library — plain HTTP POST to Claude API (no SDK needed)
encoding/json             — parse response
```

### Test Cases

**Unit — prompt builder**
- `TestPromptIncludesAllComments` — 4 comments passed in, prompt contains all 4 comment bodies
- `TestPromptIncludesCLAUDEMD` — CLAUDE.md found in repo path, its content appears in prompt
- `TestPromptWithoutCLAUDEMD` — no CLAUDE.md found, prompt builds successfully without it
- `TestPromptTruncatesLargeDiff` — diff over 8000 tokens gets truncated, truncation note is present in prompt
- `TestPromptStructuredOutputRequest` — prompt explicitly asks for JSON array response

**Unit — response parser**
- `TestParserExtractsAllVerdicts` — Claude returns JSON with 4 verdicts, parser returns 4 `TriageResult` structs
- `TestParserHandlesFixVerdict` — verdict string "fix" maps to `VerdictFix` constant
- `TestParserHandlesYourCallVerdict` — verdict "your-call" maps correctly
- `TestParserHandlesSkipVerdict` — verdict "skip" maps correctly
- `TestParserInvalidJSON` — Claude returns malformed JSON, parser returns clear error (not a panic)
- `TestParserMissingCommentID` — verdict entry missing `comment_id`, parser returns error

**Unit — triage orchestrator**
- `TestTriageUpdatesCommentStates` — after triage, all comments in DB have `triage_verdict` set
- `TestTriageHandlesAPIError` — Claude API returns 500, triage returns error, no DB writes

**Integration (requires real Anthropic API key)**
- `TestTriageRealAPI` — send 3 real comments (one clear bug, one style nit, one ambiguous), assert verdicts are reasonable

---

## Step 7 — Queue Manager

**Goal:** A clean state machine that moves comments through their lifecycle and exposes simple functions for the rest of the system to use.

### What to Build
- `internal/queue/queue.go`:
  - `Enqueue(commentIDs []string)` — moves comments from `triaged` to `queued`
  - `Park(commentIDs []string)` — moves `queued` to `parked` (session is busy)
  - `Unpark(branch string)` — moves `parked` back to `queued` when session becomes free
  - `MarkInProgress(commentIDs []string)` — moves `queued` to `in_progress`
  - `MarkDone(commentIDs []string, commitHash string)` — moves to `done`, records commit
  - `ListQueued(sessionID string)` — returns all `queued` comments for a session
  - `ListParked(sessionID string)` — returns all `parked` comments for a session

### Test Cases

**Unit — state transitions**
- `TestEnqueueMovesFromTriagedToQueued` — 3 triaged comments enqueued, all have state `queued`
- `TestEnqueueRejectsWrongState` — trying to enqueue a `done` comment returns error
- `TestParkMovesFromQueuedToParked` — queued comments parked, all have state `parked`
- `TestUnparkRestoresQueuedState` — parked comments for branch unparked, back to `queued`
- `TestUnparkOnlyAffectsCorrectBranch` — branch A has parked comments, unpacking branch B does not affect branch A
- `TestMarkInProgress` — queued comment marked in_progress
- `TestMarkDone` — in_progress comment marked done, commit hash saved
- `TestListQueued` — 5 comments across 2 sessions, list for session A returns only its comments
- `TestListParked` — same isolation as above

**Unit — edge cases**
- `TestDoubleEnqueue` — enqueuing already-queued comment is a no-op, no error, no duplicate
- `TestMarkDoneWithEmptyCommitHash` — commit hash is optional, empty string is accepted

---

## Step 8 — Claude Runner

**Goal:** Invoke `claude --resume <session-id> -p "<prompt>"` as a subprocess, capture output, and handle all process lifecycle edge cases.

### What to Build
- `internal/runner/claude.go`:
  - `Run(sessionID string, prompt string) (*RunResult, error)` — builds and executes the claude command
  - `IsClaudeInstalled() bool` — checks if `claude` CLI is on PATH
  - `IsSessionActive(sessionID string) bool` — checks if a `claude` process with that session is currently running (using process list)
- `RunResult` struct: `Output string`, `CommitHash string` (extracted from output if claude made a commit), `Duration time.Duration`
- Commit hash extraction: scan Claude's output for git commit patterns
- Timeout: configurable, default 10 minutes — kill the subprocess if it hangs

### Test Cases

**Unit — claude binary detection**
- `TestClaudeInstalledOnPath` — mock PATH with a fake `claude` binary, `IsClaudeInstalled()` returns true
- `TestClaudeNotOnPath` — empty PATH, returns false

**Unit — active session detection**
- `TestSessionIsActive` — mock process list includes `claude --resume abc123`, returns true for `abc123`
- `TestSessionIsNotActive` — process list has no claude processes, returns false

**Unit — commit hash extraction**
- `TestExtractCommitFromOutput` — output contains `commit a3f91bc`, extracts `a3f91bc`
- `TestExtractCommitNoCommitMade` — output has no commit reference, returns empty string, no error
- `TestExtractCommitMultipleMatches` — output has two commit references, returns the last one

**Unit — prompt builder**
- `TestRunnerPromptIncludesAllComments` — 3 approved comments, prompt contains all 3
- `TestRunnerPromptIncludesFileAndLine` — comment has file path and line number, both appear in prompt

**Integration (requires claude CLI installed)**
- `TestRunnerHeadlessExecution` — run `claude -p "say hello"` (no resume), assert exit code 0 and non-empty output
- `TestRunnerResumeExistingSession` — create a real session, resume it with a simple prompt, assert it runs

---

## Step 9 — TUI Triage Dialog

**Goal:** An interactive terminal dialog built with Bubble Tea that displays triage results, lets the developer override verdicts, and returns a final approved list.

### What to Build
- `internal/dialog/model.go` — Bubble Tea model for the triage UI:
  - Displays PR info at top (PR number, branch, repo)
  - Lists each comment with verdict icon (✅ / ⚠️ / ❌), author, file:line, body preview
  - Arrow keys to navigate, Space to toggle override, Enter to confirm, Q to dismiss
  - Shows counts: "2 fix · 1 your-call · 1 skip"
  - "Approve selected" and "Later" buttons
- `internal/dialog/render.go` — Bubble Tea view rendering
- `internal/dialog/dialog.go` — entry point: `Show(comments []TriageResult) ([]string, error)` returns approved comment IDs or error if dismissed

### Dependencies to Add
```
charmbracelet/bubbletea    — TUI framework
charmbracelet/lipgloss     — terminal styling
```

### Test Cases

**Unit — model logic (no rendering)**
- `TestDialogInitialState` — model initialized with 4 comments, 2 fix + 1 your-call + 1 skip, counts are correct
- `TestDialogToggleOverride` — navigate to a "skip" comment, press Space, verdict becomes "fix"
- `TestDialogToggleOverrideBack` — toggle twice, returns to original verdict
- `TestDialogApproveReturnsFixedComments` — press Enter on Approve, returns only comments with verdict "fix"
- `TestDialogApproveExcludesSkipped` — skip-verdict comments not in returned list
- `TestDialogDismissReturnsEmpty` — press Q or select "Later", returns empty list and nil error
- `TestDialogEmptyInput` — no comments passed in, model shows "No new comments" state

**Manual check (visual)**
- Run `reviewbridge queue --preview` to launch the dialog with mock data and verify it looks correct in terminal

---

## Step 10 — Notification System

**Goal:** Notify the developer when new triaged comments are ready, using desktop notifications and a terminal fallback.

### What to Build
- `internal/notify/notify.go`:
  - `Notify(title, body string)` — sends desktop notification (macOS, Linux, Windows)
  - `NotifyComments(pr PullRequest, results []TriageResult)` — formats a comment-specific notification
  - Terminal fallback: if desktop notification fails, print a visible banner to stdout
- Notification format:
  ```
  ReviewBridge — PR #47 (feature/issue-a)
  3 comments triaged: ✅ 2 fix · ⚠️ 1 your-call
  ```

### Dependencies to Add
```
gen2brain/beeep   — cross-platform desktop notifications
```

### Test Cases

**Unit — message formatter**
- `TestNotifyFormatsSingleFix` — 1 fix comment, message says "1 fix"
- `TestNotifyFormatsMultipleVerdicts` — 2 fix + 1 your-call + 1 skip, all three appear in message
- `TestNotifyFormatsAllSkip` — all comments are skip, message says "nothing to fix"

**Unit — fallback behavior**
- `TestNotifyFallsBackToTerminal` — mock desktop notification as failing, assert terminal output is produced
- `TestNotifyDoesNotPanicOnEmptyComments` — empty list passed, no panic, no notification sent

**Manual check**
- Run `reviewbridge notify --test` and verify a desktop notification appears

---

## Step 11 — Daemon Orchestrator

**Goal:** Tie all previous components into a single running daemon with a main loop, graceful shutdown, and proper startup sequence.

### What to Build
- `internal/daemon/daemon.go` — main orchestrator:
  - Startup sequence:
    1. Load config
    2. Initialize DB (run migrations)
    3. Start session watcher
    4. Run startup catch-up poll for all open PRs/MRs
    5. Start polling loop (ticker)
  - Main loop:
    1. Poll platforms for new comments
    2. Triage new comments
    3. Send notification
    4. Wait for user input (via dialog or `/check-reviews` queue file)
    5. On approval: check if session is active, route accordingly
    6. Run Claude runner or park comments
  - Graceful shutdown on SIGINT / SIGTERM: finish current poll, close DB, exit cleanly
- PID file at `~/.reviewbridge/daemon.pid` so `reviewbridge stop` can find and kill it
- `internal/daemon/watcher.go` — session watcher integration (calls session tracker)

### Test Cases

**Unit — startup sequence**
- `TestDaemonStartupInitializesDB` — daemon start creates DB file if not exists
- `TestDaemonStartupRunsCatchUp` — on startup, catch-up poll is called before first interval poll
- `TestDaemonStartupWithCorruptConfig` — corrupt config file, daemon exits with clear error message

**Unit — routing logic**
- `TestDaemonRoutesToCorrectSession` — 2 sessions (branch A and B), comments arrive for branch A, runner is called with session A's ID
- `TestDaemonParksWhenSessionActive` — session is active (running interactively), comments are parked not immediately run
- `TestDaemonUnparksWhenSessionFrees` — session transitions from active to idle, parked comments are unparked and processed

**Unit — graceful shutdown**
- `TestDaemonShutdownOnSIGINT` — send SIGINT, daemon stops within 3s, DB is closed cleanly
- `TestDaemonPIDFileWrittenOnStart` — PID file exists after daemon starts
- `TestDaemonPIDFileRemovedOnStop` — PID file deleted after daemon stops

**Integration — full loop (no real API, mocked platform)**
- `TestDaemonFullLoopMocked` — mock platform returns 2 comments, triage mock returns "fix" for both, runner mock is called with correct session ID and prompt

---

## Step 12 — CLI Commands

**Goal:** All CLI commands are fully functional and give the developer clear feedback at every step.

### What to Build
- `reviewbridge init` — guided setup: prompts for API keys, validates them, writes config
- `reviewbridge start` — starts daemon as background process, writes PID file, confirms running
- `reviewbridge stop` — reads PID file, sends SIGTERM, waits for exit, confirms stopped
- `reviewbridge status` — reads DB, shows all tracked sessions and their linked PRs/MRs
- `reviewbridge queue` — shows all pending/parked comments organized by PR
- `reviewbridge link` — manually link a session to a branch or PR

### Test Cases

**Unit — init command**
- `TestInitWritesConfigFile` — after init with valid inputs, config file exists at correct path
- `TestInitValidatesAnthropicKey` — mock Anthropic API returns 401, init shows error and re-prompts
- `TestInitValidatesGitHubToken` — mock GitHub API returns 401, init shows error
- `TestInitSkipsGitLabIfEmpty` — user presses Enter on GitLab token, config has no GitLab section

**Unit — start/stop**
- `TestStartWritesPIDFile` — after start, PID file exists
- `TestStartFailsIfAlreadyRunning` — PID file already exists, start shows "already running"
- `TestStopKillsDaemon` — start daemon, then stop, PID file removed and process gone
- `TestStopFailsIfNotRunning` — no PID file, stop shows "not running" cleanly

**Unit — status command**
- `TestStatusShowsAllSessions` — 3 sessions in DB, all 3 appear in output
- `TestStatusShowsLinkedPRs` — session linked to PR, PR number and platform appear in output
- `TestStatusEmptyState` — no sessions in DB, shows friendly "no sessions tracked" message

**Unit — queue command**
- `TestQueueShowsGroupedByPR` — 4 queued comments across 2 PRs, output shows 2 groups
- `TestQueueShowsTriageVerdicts` — verdict icons (✅ ⚠️ ❌) appear next to comments
- `TestQueueEmptyState` — no queued comments, shows "queue is empty"

---

## Step 13 — /check-reviews Slash Command

**Goal:** A Claude Code slash command that pulls pending comments for the current branch into the active session so the user can handle them interactively.

### What to Build
- `skills/check-reviews.md` — the slash command definition:
  - Reads current git branch
  - Reads `~/.reviewbridge/queue/<branch>.json` (queue file written by the daemon)
  - If empty: reports "no pending review comments"
  - If not empty: lists all pending comments and asks Claude to fix them within the current session
- Queue file writer in the daemon: when comments are queued, daemon writes a JSON snapshot to `~/.reviewbridge/queue/<branch>.json` so the slash command can read it
- Install command: `reviewbridge install-skill` copies the skill file to `~/.claude/commands/`

### Test Cases

**Unit — queue file writer**
- `TestQueueFileWrittenOnEnqueue` — after enqueue, JSON file exists at expected path for the branch
- `TestQueueFileUpdatedOnStateChange` — when a comment moves to `done`, it is removed from the queue file
- `TestQueueFileCreatesDirectoryIfMissing` — `~/.reviewbridge/queue/` doesn't exist, writer creates it

**Unit — install skill command**
- `TestInstallSkillCopiesFile` — `reviewbridge install-skill` copies skill to `~/.claude/commands/check-reviews.md`
- `TestInstallSkillOverwritesExisting` — skill already exists, install overwrites with latest version
- `TestInstallSkillCreatesDirectoryIfMissing` — `~/.claude/commands/` doesn't exist, install creates it

**Manual check**
- In a Claude Code session on a branch with queued comments, type `/check-reviews` and verify comments appear

---

## Step 14 — End-to-End Tests

**Goal:** Simulate a realistic developer workflow from session open to comment fixed, using Docker-based mock platforms and a real (but sandboxed) Claude runner.

### Docker Setup for E2E

All E2E tests run against the WireMock containers from `docker-compose.test.yml`. A helper script `tests/e2e/setup.sh` starts the containers, waits for them to be healthy, runs the tests, then tears them down:

```bash
#!/bin/sh
# tests/e2e/setup.sh
docker compose -f docker-compose.test.yml up -d
until curl -sf http://localhost:8080/__admin/health > /dev/null; do sleep 1; done
until curl -sf http://localhost:8081/__admin/health > /dev/null; do sleep 1; done

REVIEWBRIDGE_GITHUB_BASE_URL=http://localhost:8080 \
REVIEWBRIDGE_GITLAB_BASE_URL=http://localhost:8081 \
go test ./tests/e2e/... -v -timeout 120s

docker compose -f docker-compose.test.yml down
```

Run all E2E tests with:
```bash
sh tests/e2e/setup.sh
```

Each E2E test resets WireMock stubs before running via the admin API (`POST /__admin/reset`) so tests are fully isolated from each other.

### Test Cases

**E2E — GitHub happy path**
1. Start daemon with mock GitHub platform and mock Claude runner
2. Open a fake Claude Code session JSONL at the watched path, branch: `feature/test`
3. Mock GitHub reports PR #1 on branch `feature/test` with 2 new comments
4. Assert: daemon fetches comments, saves to DB as `fetched`
5. Assert: triage is called with the diff and comments
6. Assert: notification is sent
7. Simulate user approving both comments in dialog
8. Assert: Claude runner is called with `--resume <session-id>` and prompt containing both comments
9. Assert: comments are updated to `done` in DB

**E2E — GitLab happy path**
- Same flow but with mock GitLab platform (MR instead of PR, "notes" instead of "comments")
- Assert same outcome

**E2E — offline catch-up**
1. Pre-populate DB with a PR that has `last_checked_at` = 4 hours ago
2. Mock platform returns 3 comments from that 4-hour window
3. Start daemon
4. Assert: all 3 comments are fetched immediately on startup (before first poll interval)

**E2E — session mismatch (issue A while working on C)**
1. Two sessions in DB: `abc123` on `feature/issue-a`, `xyz789` on `feature/issue-c`
2. Mock: `xyz789` is currently active (process running)
3. Comments arrive for `feature/issue-a`
4. Assert: triage runs
5. User approves
6. Assert: runner called with `abc123` (not `xyz789`)
7. Assert: `xyz789` session is undisturbed

**E2E — session busy, comments parked**
1. One session `abc123` on `feature/issue-a`, marked as active
2. Comments arrive for `feature/issue-a`
3. User approves in dialog
4. Assert: comments are parked (not immediately run) because session is active
5. Simulate session going idle
6. Assert: parked comments are unparked and runner is called automatically

**E2E — daemon restart, no comments lost**
1. Start daemon, mock platform returns 3 comments, daemon fetches and saves them
2. Stop daemon before user approves
3. Restart daemon
4. Assert: 3 comments still in DB as `queued` (not lost, not refetched)

---

## Step 15 — Distribution

**Goal:** Anyone can install ReviewBridge with a single command. Binaries are built reproducibly via Docker.

### What to Build
- `Makefile` with targets: `build`, `build-docker`, `test`, `test-integration`, `test-e2e`, `release`, `install`
- Cross-compilation via `Dockerfile.build` for all targets: `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`
- GitHub Actions CI workflow (`.github/workflows/ci.yml`):
  - On every push: `go test ./...` (unit tests only, no Docker needed)
  - On every push: `docker compose -f docker-compose.test.yml up -d && go test ./tests/integration/...`
  - On tag push: build all platform binaries using `Dockerfile.build` and create GitHub Release
- Homebrew formula `Formula/reviewbridge.rb`:
  - Downloads the correct binary for the user's platform
  - Adds `reviewbridge` to PATH
  - Runs `reviewbridge init` on first install (via `post_install`)
- `reviewbridge update` command — checks GitHub releases API for newer version, prints update instructions

### Makefile Targets

```makefile
build:
	go build -o ./bin/reviewbridge ./cmd/reviewbridge

build-docker:
	docker build -f Dockerfile.build \
	  --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 \
	  -o ./bin/ .

test:
	go test ./...

test-integration:
	docker compose -f docker-compose.test.yml up -d
	REVIEWBRIDGE_GITHUB_BASE_URL=http://localhost:8080 \
	REVIEWBRIDGE_GITLAB_BASE_URL=http://localhost:8081 \
	go test ./tests/integration/...
	docker compose -f docker-compose.test.yml down

test-e2e:
	sh tests/e2e/setup.sh

release:
	@for platform in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do \
	  OS=$$(echo $$platform | cut -d/ -f1); \
	  ARCH=$$(echo $$platform | cut -d/ -f2); \
	  docker build -f Dockerfile.build \
	    --build-arg TARGETOS=$$OS --build-arg TARGETARCH=$$ARCH \
	    -o ./dist/reviewbridge-$$OS-$$ARCH . ; \
	done
```

### Test Cases

**Build tests**
- `TestBuildCompilesDarwinAMD64` — `GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build` exits 0
- `TestBuildCompilesDarwinARM64` — `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build` exits 0
- `TestBuildCompilesLinuxAMD64` — `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build` exits 0
- `TestNoCGODependencies` — `CGO_ENABLED=0 go build` exits 0 (confirms pure Go SQLite, no CGO)
- `TestDockerBuildSucceeds` — `docker build -f Dockerfile.build .` exits 0

**CI workflow checks**
- Push to branch → unit tests run automatically, no Docker credentials needed
- Push to branch → integration tests run using WireMock containers, no real tokens needed
- Push a git tag → all 4 platform binaries are built and attached to GitHub Release

**Manual install check**
- `brew install reviewbridge` completes without error
- `reviewbridge --version` prints a version string
- `reviewbridge --help` shows all commands

---

## Overall Test Coverage Targets

| Layer | Target | Type |
|---|---|---|
| DB queries | 100% | Unit |
| Session tracker | 90% | Unit + Integration |
| Platform clients | 85% | Unit (mocked HTTP) + Integration |
| Triage engine | 85% | Unit + Integration |
| Queue manager | 100% | Unit |
| Claude runner | 80% | Unit + Integration |
| TUI dialog | 75% | Unit (model logic) |
| Daemon orchestrator | 80% | Unit + Integration |
| CLI commands | 85% | Unit |
| End-to-end | Full happy paths + key edge cases | E2E |

---

## Running Tests

```bash
# Unit tests — no Docker, no credentials, always fast
go test ./...

# Unit tests with coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Integration tests — uses Docker WireMock, no real credentials needed
make test-integration
# or manually:
docker compose -f docker-compose.test.yml up -d
REVIEWBRIDGE_GITHUB_BASE_URL=http://localhost:8080 \
REVIEWBRIDGE_GITLAB_BASE_URL=http://localhost:8081 \
go test ./tests/integration/...
docker compose -f docker-compose.test.yml down

# Integration tests — real credentials (optional)
REVIEWBRIDGE_INTEGRATION=true GITHUB_TOKEN=ghp_... go test ./tests/integration/ -run TestRealAPI

# End-to-end tests — uses Docker WireMock + real Claude CLI
make test-e2e
# or manually:
sh tests/e2e/setup.sh

# Single component
go test ./internal/triage/...
go test ./internal/db/...

# Run daemon in Docker (dev mode)
docker compose -f docker-compose.dev.yml up
```

---

## Definition of Done Per Step

A step is complete when:
1. All test cases listed for that step pass with `go test`
2. No new linter errors (`golangci-lint run`)
3. The component can be demonstrated working in isolation (not just tests)
4. Code is committed with a descriptive message
