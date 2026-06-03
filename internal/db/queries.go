package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("not found")

func (d *DB) SaveSession(s *Session) error {
	_, err := d.sql.Exec(
		`INSERT INTO sessions (session_id, repo_path, branch_name, last_active_at, status)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		   repo_path      = excluded.repo_path,
		   branch_name    = excluded.branch_name,
		   last_active_at = excluded.last_active_at,
		   status         = excluded.status`,
		s.SessionID, s.RepoPath, s.BranchName, timeToStr(s.LastActiveAt), s.Status,
	)
	return err
}

func (d *DB) GetSession(sessionID string) (*Session, error) {
	row := d.sql.QueryRow(
		`SELECT session_id, repo_path, branch_name, last_active_at, status
		 FROM sessions WHERE session_id = ?`,
		sessionID,
	)
	return scanSession(row)
}

func (d *DB) GetSessionByBranch(branchName string) (*Session, error) {
	row := d.sql.QueryRow(
		`SELECT session_id, repo_path, branch_name, last_active_at, status
		 FROM sessions WHERE branch_name = ?
		 ORDER BY last_active_at DESC LIMIT 1`,
		branchName,
	)
	return scanSession(row)
}

func (d *DB) ListActiveSessions() ([]*Session, error) {
	rows, err := d.sql.Query(
		`SELECT session_id, repo_path, branch_name, last_active_at, status
		 FROM sessions WHERE status != ?`,
		SessionStatusClosed,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (d *DB) SavePullRequest(pr *PullRequest) error {
	var sessionID interface{}
	if pr.SessionID != nil {
		sessionID = *pr.SessionID
	}
	_, err := d.sql.Exec(
		`INSERT INTO pull_requests (pr_id, platform, repo, branch_name, session_id, last_checked_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(pr_id) DO UPDATE SET
		   platform        = excluded.platform,
		   repo            = excluded.repo,
		   branch_name     = excluded.branch_name,
		   session_id      = excluded.session_id,
		   last_checked_at = excluded.last_checked_at,
		   status          = excluded.status`,
		pr.PRID, pr.Platform, pr.Repo, pr.BranchName, sessionID,
		timeToStr(pr.LastCheckedAt), pr.Status,
	)
	return err
}

func (d *DB) GetPullRequest(prID string) (*PullRequest, error) {
	row := d.sql.QueryRow(
		`SELECT pr_id, platform, repo, branch_name, session_id, last_checked_at, status
		 FROM pull_requests WHERE pr_id = ?`,
		prID,
	)
	return scanPullRequest(row)
}

func (d *DB) ListOpenPullRequests() ([]*PullRequest, error) {
	rows, err := d.sql.Query(
		`SELECT pr_id, platform, repo, branch_name, session_id, last_checked_at, status
		 FROM pull_requests WHERE status = ?`,
		PRStatusOpen,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPullRequests(rows)
}

func (d *DB) UpdateLastChecked(prID string, t time.Time) error {
	res, err := d.sql.Exec(
		`UPDATE pull_requests SET last_checked_at = ? WHERE pr_id = ?`,
		timeToStr(t), prID,
	)
	if err != nil {
		return err
	}
	return expectOneRow(res, prID)
}

func (d *DB) LinkPRToSession(prID, sessionID string) error {
	res, err := d.sql.Exec(
		`UPDATE pull_requests SET session_id = ? WHERE pr_id = ?`,
		sessionID, prID,
	)
	if err != nil {
		return err
	}
	return expectOneRow(res, prID)
}

func (d *DB) SaveComment(c *Comment) error {
	_, err := d.sql.Exec(
		`INSERT INTO comments
		   (comment_id, pr_id, author, body, file_path, line_number, created_at, fetched_at, triage_verdict, state, commit_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(comment_id) DO UPDATE SET
		   pr_id          = excluded.pr_id,
		   author         = excluded.author,
		   body           = excluded.body,
		   file_path      = excluded.file_path,
		   line_number    = excluded.line_number,
		   created_at     = excluded.created_at,
		   fetched_at     = excluded.fetched_at,
		   triage_verdict = excluded.triage_verdict,
		   state          = excluded.state,
		   commit_hash    = excluded.commit_hash`,
		c.CommentID, c.PRID, c.Author, c.Body, c.FilePath, c.LineNumber,
		timeToStr(c.CreatedAt), timeToStr(c.FetchedAt), c.TriageVerdict, c.State, c.CommitHash,
	)
	return err
}

func (d *DB) GetComment(commentID string) (*Comment, error) {
	row := d.sql.QueryRow(
		`SELECT comment_id, pr_id, author, body, file_path, line_number,
		        created_at, fetched_at, triage_verdict, state, commit_hash
		 FROM comments WHERE comment_id = ?`,
		commentID,
	)
	return scanComment(row)
}

func (d *DB) MarkCommentDone(commentID, commitHash string) error {
	res, err := d.sql.Exec(
		`UPDATE comments SET state = ?, commit_hash = ? WHERE comment_id = ?`,
		CommentStateDone, commitHash, commentID,
	)
	if err != nil {
		return err
	}
	return expectOneRow(res, commentID)
}

func (d *DB) ListCommentsByStateAndSession(state, sessionID string) ([]*Comment, error) {
	rows, err := d.sql.Query(
		`SELECT c.comment_id, c.pr_id, c.author, c.body, c.file_path, c.line_number,
		        c.created_at, c.fetched_at, c.triage_verdict, c.state, c.commit_hash
		 FROM comments c
		 JOIN pull_requests pr ON c.pr_id = pr.pr_id
		 WHERE c.state = ? AND pr.session_id = ?`,
		state, sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanComments(rows)
}

func (d *DB) ListCommentsByStateAndBranch(state, branchName string) ([]*Comment, error) {
	rows, err := d.sql.Query(
		`SELECT c.comment_id, c.pr_id, c.author, c.body, c.file_path, c.line_number,
		        c.created_at, c.fetched_at, c.triage_verdict, c.state, c.commit_hash
		 FROM comments c
		 JOIN pull_requests pr ON c.pr_id = pr.pr_id
		 JOIN sessions s ON pr.session_id = s.session_id
		 WHERE c.state = ? AND s.branch_name = ?`,
		state, branchName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanComments(rows)
}

func (d *DB) SetTriageResult(commentID, verdict string) error {
	res, err := d.sql.Exec(
		`UPDATE comments SET triage_verdict = ?, state = ? WHERE comment_id = ?`,
		verdict, CommentStateTriaged, commentID,
	)
	if err != nil {
		return err
	}
	return expectOneRow(res, commentID)
}

func (d *DB) UpdateCommentState(commentID, state string) error {
	res, err := d.sql.Exec(
		`UPDATE comments SET state = ? WHERE comment_id = ?`,
		state, commentID,
	)
	if err != nil {
		return err
	}
	return expectOneRow(res, commentID)
}

func (d *DB) ListCommentsByPR(prID string) ([]*Comment, error) {
	rows, err := d.sql.Query(
		`SELECT comment_id, pr_id, author, body, file_path, line_number,
		        created_at, fetched_at, triage_verdict, state, commit_hash
		 FROM comments WHERE pr_id = ?`,
		prID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanComments(rows)
}

func (d *DB) ListQueuedComments() ([]*Comment, error) {
	return d.ListCommentsByState(CommentStateQueued)
}

func (d *DB) ListCommentsByState(state string) ([]*Comment, error) {
	rows, err := d.sql.Query(
		`SELECT comment_id, pr_id, author, body, file_path, line_number,
		        created_at, fetched_at, triage_verdict, state, commit_hash
		 FROM comments WHERE state = ?`,
		state,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanComments(rows)
}

func scanSession(row *sql.Row) (*Session, error) {
	var s Session
	var lastActiveAt string
	err := row.Scan(&s.SessionID, &s.RepoPath, &s.BranchName, &lastActiveAt, &s.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.LastActiveAt = strToTime(lastActiveAt)
	return &s, nil
}

func scanSessions(rows *sql.Rows) ([]*Session, error) {
	var out []*Session
	for rows.Next() {
		var s Session
		var lastActiveAt string
		if err := rows.Scan(&s.SessionID, &s.RepoPath, &s.BranchName, &lastActiveAt, &s.Status); err != nil {
			return nil, err
		}
		s.LastActiveAt = strToTime(lastActiveAt)
		out = append(out, &s)
	}
	return out, rows.Err()
}

func scanPullRequest(row *sql.Row) (*PullRequest, error) {
	var pr PullRequest
	var lastCheckedAt string
	var sessionID sql.NullString
	err := row.Scan(&pr.PRID, &pr.Platform, &pr.Repo, &pr.BranchName, &sessionID, &lastCheckedAt, &pr.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sessionID.Valid {
		pr.SessionID = &sessionID.String
	}
	pr.LastCheckedAt = strToTime(lastCheckedAt)
	return &pr, nil
}

func scanPullRequests(rows *sql.Rows) ([]*PullRequest, error) {
	var out []*PullRequest
	for rows.Next() {
		var pr PullRequest
		var lastCheckedAt string
		var sessionID sql.NullString
		if err := rows.Scan(&pr.PRID, &pr.Platform, &pr.Repo, &pr.BranchName, &sessionID, &lastCheckedAt, &pr.Status); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			pr.SessionID = &sessionID.String
		}
		pr.LastCheckedAt = strToTime(lastCheckedAt)
		out = append(out, &pr)
	}
	return out, rows.Err()
}

func scanComment(row *sql.Row) (*Comment, error) {
	var c Comment
	var createdAt, fetchedAt string
	err := row.Scan(
		&c.CommentID, &c.PRID, &c.Author, &c.Body, &c.FilePath, &c.LineNumber,
		&createdAt, &fetchedAt, &c.TriageVerdict, &c.State, &c.CommitHash,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt = strToTime(createdAt)
	c.FetchedAt = strToTime(fetchedAt)
	return &c, nil
}

func scanComments(rows *sql.Rows) ([]*Comment, error) {
	var out []*Comment
	for rows.Next() {
		var c Comment
		var createdAt, fetchedAt string
		if err := rows.Scan(
			&c.CommentID, &c.PRID, &c.Author, &c.Body, &c.FilePath, &c.LineNumber,
			&createdAt, &fetchedAt, &c.TriageVerdict, &c.State, &c.CommitHash,
		); err != nil {
			return nil, err
		}
		c.CreatedAt = strToTime(createdAt)
		c.FetchedAt = strToTime(fetchedAt)
		out = append(out, &c)
	}
	return out, rows.Err()
}

func expectOneRow(res sql.Result, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
}
