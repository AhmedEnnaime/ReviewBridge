CREATE TABLE IF NOT EXISTS sessions (
    session_id     TEXT PRIMARY KEY,
    repo_path      TEXT NOT NULL,
    branch_name    TEXT NOT NULL,
    last_active_at TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS pull_requests (
    pr_id           TEXT PRIMARY KEY,
    platform        TEXT NOT NULL,
    repo            TEXT NOT NULL,
    branch_name     TEXT NOT NULL,
    session_id      TEXT REFERENCES sessions(session_id),
    last_checked_at TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open'
);

CREATE TABLE IF NOT EXISTS comments (
    comment_id     TEXT PRIMARY KEY,
    pr_id          TEXT NOT NULL REFERENCES pull_requests(pr_id),
    author         TEXT NOT NULL,
    body           TEXT NOT NULL,
    file_path      TEXT NOT NULL DEFAULT '',
    line_number    INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL,
    fetched_at     TEXT NOT NULL,
    triage_verdict TEXT NOT NULL DEFAULT 'pending',
    state          TEXT NOT NULL DEFAULT 'fetched'
);
