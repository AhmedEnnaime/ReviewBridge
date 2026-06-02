package db

import "time"

const (
	SessionStatusActive = "active"
	SessionStatusIdle   = "idle"
	SessionStatusClosed = "closed"

	PRStatusOpen   = "open"
	PRStatusClosed = "closed"
	PRStatusMerged = "merged"

	CommentStateFetched    = "fetched"
	CommentStateTriaged    = "triaged"
	CommentStateQueued     = "queued"
	CommentStateParked     = "parked"
	CommentStateInProgress = "in_progress"
	CommentStateDone       = "done"

	VerdictFix      = "fix"
	VerdictYourCall = "your-call"
	VerdictSkip     = "skip"
	VerdictPending  = "pending"
)

type Session struct {
	SessionID    string
	RepoPath     string
	BranchName   string
	LastActiveAt time.Time
	Status       string
}

type PullRequest struct {
	PRID          string
	Platform      string
	Repo          string
	BranchName    string
	SessionID     *string
	LastCheckedAt time.Time
	Status        string
}

type Comment struct {
	CommentID     string
	PRID          string
	Author        string
	Body          string
	FilePath      string
	LineNumber    int
	CreatedAt     time.Time
	FetchedAt     time.Time
	TriageVerdict string
	State         string
}
