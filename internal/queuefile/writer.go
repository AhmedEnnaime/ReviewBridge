package queuefile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

// QueueFile is the JSON structure written to disk for each branch.
type QueueFile struct {
	Branch    string         `json:"branch"`
	UpdatedAt time.Time      `json:"updated_at"`
	Comments  []CommentEntry `json:"comments"`
}

// CommentEntry is a single comment in the queue file.
type CommentEntry struct {
	CommentID     string `json:"comment_id"`
	PRID          string `json:"pr_id"`
	Author        string `json:"author"`
	Body          string `json:"body"`
	FilePath      string `json:"file_path"`
	LineNumber    int    `json:"line_number"`
	TriageVerdict string `json:"triage_verdict"`
	State         string `json:"state"`
}

// Writer writes and maintains per-branch queue JSON files.
type Writer struct {
	dir string
	db  *db.DB
}

func New(dir string, database *db.DB) *Writer {
	return &Writer{dir: dir, db: database}
}

// SyncForComment looks up the branch for commentID and rewrites that branch's file.
func (w *Writer) SyncForComment(commentID string) error {
	c, err := w.db.GetComment(commentID)
	if err != nil || c == nil {
		return err
	}
	pr, err := w.db.GetPullRequest(c.PRID)
	if err != nil || pr == nil {
		return err
	}

	all, err := w.db.ListCommentsByPR(pr.PRID)
	if err != nil {
		return err
	}

	var pending []*db.Comment
	for _, cm := range all {
		if cm.State == db.CommentStateQueued || cm.State == db.CommentStateParked {
			pending = append(pending, cm)
		}
	}

	return w.writeBranchFile(pr.BranchName, pending)
}

// SyncBranch rewrites the queue file for a branch by scanning all open PRs on it.
func (w *Writer) SyncBranch(branch string) error {
	prs, err := w.db.ListOpenPullRequests()
	if err != nil {
		return err
	}

	var pending []*db.Comment
	for _, pr := range prs {
		if pr.BranchName != branch {
			continue
		}
		comments, err := w.db.ListCommentsByPR(pr.PRID)
		if err != nil {
			continue
		}
		for _, c := range comments {
			if c.State == db.CommentStateQueued || c.State == db.CommentStateParked {
				pending = append(pending, c)
			}
		}
	}

	return w.writeBranchFile(branch, pending)
}

func (w *Writer) writeBranchFile(branch string, comments []*db.Comment) error {
	path := w.FilePath(branch)

	if len(comments) == 0 {
		os.Remove(path) //nolint:errcheck
		return nil
	}

	if err := os.MkdirAll(w.dir, 0755); err != nil {
		return fmt.Errorf("create queue dir: %w", err)
	}

	entries := make([]CommentEntry, len(comments))
	for i, c := range comments {
		entries[i] = CommentEntry{
			CommentID:     c.CommentID,
			PRID:          c.PRID,
			Author:        c.Author,
			Body:          c.Body,
			FilePath:      c.FilePath,
			LineNumber:    c.LineNumber,
			TriageVerdict: c.TriageVerdict,
			State:         c.State,
		}
	}

	qf := QueueFile{
		Branch:    branch,
		UpdatedAt: time.Now(),
		Comments:  entries,
	}

	data, err := json.MarshalIndent(qf, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// FilePath returns the queue file path for a branch name.
// Forward slashes in branch names are replaced with hyphens.
func (w *Writer) FilePath(branch string) string {
	safe := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(w.dir, safe+".json")
}
