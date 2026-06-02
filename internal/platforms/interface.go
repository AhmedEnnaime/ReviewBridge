package platforms

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized: invalid or missing token")
	ErrRateLimited  = errors.New("rate limited by platform API")
	ErrNotFound     = errors.New("pull request not found")
)

type Platform interface {
	ListOpenPullRequests(repo string) ([]*PullRequest, error)
	GetPullRequest(repo string, prID int) (*PullRequest, error)
	ListCommentsSince(repo string, prID int, since time.Time) ([]*Comment, error)
	GetDiff(repo string, prID int) (string, error)
}

type PullRequest struct {
	Number       int
	Title        string
	SourceBranch string
	State        string
	HTMLURL      string
}

type Comment struct {
	ID        string
	Author    string
	Body      string
	FilePath  string
	Line      int
	CreatedAt time.Time
}
