package queuefile_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/queue"
	"github.com/ahmedennaime/reviewbridge/internal/queuefile"
)

func TestQueueFileWrittenOnEnqueue(t *testing.T) {
	database, q, w, dir := setup(t)

	seedAll(t, database, "sess1", "feature/issue-a", "github:owner/repo:1")
	seedComment(t, database, "c1", "github:owner/repo:1", db.CommentStateTriaged, db.VerdictFix)

	q.WithOnChange(syncFn(w))

	if err := q.Enqueue([]string{"c1"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	expected := filepath.Join(dir, "feature-issue-a.json")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("queue file not created at %s: %v", expected, err)
	}
}

func TestQueueFileUpdatedOnStateChange(t *testing.T) {
	database, q, w, dir := setup(t)

	seedAll(t, database, "sess1", "feature/issue-a", "github:owner/repo:1")
	seedComment(t, database, "c1", "github:owner/repo:1", db.CommentStateTriaged, db.VerdictFix)

	q.WithOnChange(syncFn(w))

	must(t, q.Enqueue([]string{"c1"}))

	// File should exist after enqueue.
	expected := filepath.Join(dir, "feature-issue-a.json")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("queue file missing after enqueue: %v", err)
	}

	must(t, q.MarkInProgress([]string{"c1"}))
	must(t, q.MarkDone([]string{"c1"}, "abc1234"))

	// File should be removed when no more pending comments.
	if _, err := os.Stat(expected); !os.IsNotExist(err) {
		t.Error("queue file should be removed after comment is done")
	}
}

func TestQueueFileCreatesDirectoryIfMissing(t *testing.T) {
	database, _, _, _ := setup(t)

	nonExistentDir := filepath.Join(t.TempDir(), "nested", "queue")
	w := queuefile.New(nonExistentDir, database)

	seedAll(t, database, "sess1", "main", "github:owner/repo:1")
	seedComment(t, database, "c1", "github:owner/repo:1", db.CommentStateQueued, db.VerdictFix)

	if err := w.SyncForComment("c1"); err != nil {
		t.Fatalf("SyncForComment: %v", err)
	}

	if _, err := os.Stat(nonExistentDir); err != nil {
		t.Fatalf("directory should have been created: %v", err)
	}
}

// --- helpers ---

func setup(t *testing.T) (*db.DB, *queue.Queue, *queuefile.Writer, string) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	dir := t.TempDir()
	w := queuefile.New(dir, d)
	q := queue.New(d)
	return d, q, w, dir
}

func syncFn(w *queuefile.Writer) func([]string) {
	return func(ids []string) {
		for _, id := range ids {
			w.SyncForComment(id) //nolint:errcheck
		}
	}
}

func seedAll(t *testing.T, d *db.DB, sessionID, branch, prID string) {
	t.Helper()
	sid := sessionID
	must(t, d.SaveSession(&db.Session{
		SessionID:    sessionID,
		RepoPath:     "/repos/app",
		BranchName:   branch,
		LastActiveAt: time.Now(),
		Status:       db.SessionStatusActive,
	}))
	must(t, d.SavePullRequest(&db.PullRequest{
		PRID:          prID,
		Platform:      "github",
		Repo:          "owner/repo",
		BranchName:    branch,
		SessionID:     &sid,
		LastCheckedAt: time.Now(),
		Status:        db.PRStatusOpen,
	}))
}

func seedComment(t *testing.T, d *db.DB, id, prID, state, verdict string) {
	t.Helper()
	must(t, d.SaveComment(&db.Comment{
		CommentID:     id,
		PRID:          prID,
		Author:        "reviewer",
		Body:          "Some review comment",
		FilePath:      "main.go",
		LineNumber:    42,
		CreatedAt:     time.Now(),
		FetchedAt:     time.Now(),
		TriageVerdict: verdict,
		State:         state,
	}))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
