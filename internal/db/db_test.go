package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func truncSec(t time.Time) time.Time {
	return t.UTC().Truncate(time.Second)
}

func TestMigrationsRunOnEmptyDB(t *testing.T) {
	d := newTestDB(t)
	if d == nil {
		t.Fatal("expected non-nil DB")
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	d.Close()

	d2, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer d2.Close()
}

func TestSchemaHasAllTables(t *testing.T) {
	d := newTestDB(t)
	for _, table := range []string{"sessions", "pull_requests", "comments"} {
		if !d.TableExists(table) {
			t.Errorf("table %q not found in schema", table)
		}
	}
}

func TestSaveAndGetSession(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	s := &db.Session{
		SessionID:    "abc123",
		RepoPath:     "/repos/myapp",
		BranchName:   "feature/issue-a",
		LastActiveAt: now,
		Status:       db.SessionStatusActive,
	}
	if err := d.SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := d.GetSession("abc123")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got == nil {
		t.Fatal("expected session, got nil")
	}
	if got.SessionID != s.SessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, s.SessionID)
	}
	if got.RepoPath != s.RepoPath {
		t.Errorf("RepoPath = %q, want %q", got.RepoPath, s.RepoPath)
	}
	if got.BranchName != s.BranchName {
		t.Errorf("BranchName = %q, want %q", got.BranchName, s.BranchName)
	}
	if !got.LastActiveAt.Equal(now) {
		t.Errorf("LastActiveAt = %v, want %v", got.LastActiveAt, now)
	}
	if got.Status != s.Status {
		t.Errorf("Status = %q, want %q", got.Status, s.Status)
	}
}

func TestGetSessionByBranch(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	d.SaveSession(&db.Session{SessionID: "s1", RepoPath: "/r", BranchName: "feature/a", LastActiveAt: now, Status: db.SessionStatusActive})
	d.SaveSession(&db.Session{SessionID: "s2", RepoPath: "/r", BranchName: "feature/b", LastActiveAt: now, Status: db.SessionStatusActive})

	got, err := d.GetSessionByBranch("feature/a")
	if err != nil {
		t.Fatalf("GetSessionByBranch: %v", err)
	}
	if got == nil || got.SessionID != "s1" {
		t.Errorf("got session %v, want s1", got)
	}
}

func TestGetSessionByBranch_NotFound(t *testing.T) {
	d := newTestDB(t)
	got, err := d.GetSessionByBranch("feature/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestListActiveSessions(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	d.SaveSession(&db.Session{SessionID: "s1", RepoPath: "/r", BranchName: "a", LastActiveAt: now, Status: db.SessionStatusActive})
	d.SaveSession(&db.Session{SessionID: "s2", RepoPath: "/r", BranchName: "b", LastActiveAt: now, Status: db.SessionStatusIdle})
	d.SaveSession(&db.Session{SessionID: "s3", RepoPath: "/r", BranchName: "c", LastActiveAt: now, Status: db.SessionStatusClosed})

	sessions, err := d.ListActiveSessions()
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("got %d sessions, want 2", len(sessions))
	}
}

func TestSaveAndGetPullRequest(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	pr := &db.PullRequest{
		PRID:          "github:owner/repo:12",
		Platform:      "github",
		Repo:          "owner/repo",
		BranchName:    "feature/issue-a",
		SessionID:     nil,
		LastCheckedAt: now,
		Status:        db.PRStatusOpen,
	}
	if err := d.SavePullRequest(pr); err != nil {
		t.Fatalf("SavePullRequest: %v", err)
	}
	got, err := d.GetPullRequest("github:owner/repo:12")
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if got == nil {
		t.Fatal("expected PR, got nil")
	}
	if got.PRID != pr.PRID {
		t.Errorf("PRID = %q, want %q", got.PRID, pr.PRID)
	}
	if got.Platform != pr.Platform {
		t.Errorf("Platform = %q, want %q", got.Platform, pr.Platform)
	}
	if got.BranchName != pr.BranchName {
		t.Errorf("BranchName = %q, want %q", got.BranchName, pr.BranchName)
	}
	if !got.LastCheckedAt.Equal(now) {
		t.Errorf("LastCheckedAt = %v, want %v", got.LastCheckedAt, now)
	}
	if got.SessionID != nil {
		t.Errorf("SessionID = %v, want nil", got.SessionID)
	}
}

func TestUpdateLastChecked(t *testing.T) {
	d := newTestDB(t)
	old := truncSec(time.Now().Add(-1 * time.Hour))
	d.SavePullRequest(&db.PullRequest{
		PRID: "github:owner/repo:12", Platform: "github", Repo: "owner/repo",
		BranchName: "feature/a", LastCheckedAt: old, Status: db.PRStatusOpen,
	})

	newer := truncSec(time.Now())
	if err := d.UpdateLastChecked("github:owner/repo:12", newer); err != nil {
		t.Fatalf("UpdateLastChecked: %v", err)
	}

	got, _ := d.GetPullRequest("github:owner/repo:12")
	if !got.LastCheckedAt.Equal(newer) {
		t.Errorf("LastCheckedAt = %v, want %v", got.LastCheckedAt, newer)
	}
}

func TestListOpenPullRequests(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	for _, pr := range []struct {
		id     string
		status string
	}{
		{"pr1", db.PRStatusOpen},
		{"pr2", db.PRStatusOpen},
		{"pr3", db.PRStatusOpen},
		{"pr4", db.PRStatusMerged},
	} {
		d.SavePullRequest(&db.PullRequest{
			PRID: pr.id, Platform: "github", Repo: "r", BranchName: "b",
			LastCheckedAt: now, Status: pr.status,
		})
	}

	prs, err := d.ListOpenPullRequests()
	if err != nil {
		t.Fatalf("ListOpenPullRequests: %v", err)
	}
	if len(prs) != 3 {
		t.Errorf("got %d PRs, want 3", len(prs))
	}
}

func TestLinkPRToSession(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	d.SaveSession(&db.Session{SessionID: "s1", RepoPath: "/r", BranchName: "feature/a", LastActiveAt: now, Status: db.SessionStatusActive})
	d.SavePullRequest(&db.PullRequest{
		PRID: "pr1", Platform: "github", Repo: "r", BranchName: "feature/a",
		LastCheckedAt: now, Status: db.PRStatusOpen,
	})

	if err := d.LinkPRToSession("pr1", "s1"); err != nil {
		t.Fatalf("LinkPRToSession: %v", err)
	}

	got, _ := d.GetPullRequest("pr1")
	if got.SessionID == nil || *got.SessionID != "s1" {
		t.Errorf("SessionID = %v, want s1", got.SessionID)
	}
}

func TestSaveAndGetComment(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	d.SavePullRequest(&db.PullRequest{
		PRID: "pr1", Platform: "github", Repo: "r", BranchName: "b",
		LastCheckedAt: now, Status: db.PRStatusOpen,
	})

	c := &db.Comment{
		CommentID:     "c1",
		PRID:          "pr1",
		Author:        "@john",
		Body:          "Missing null check",
		FilePath:      "main.go",
		LineNumber:    42,
		CreatedAt:     now,
		FetchedAt:     now,
		TriageVerdict: db.VerdictPending,
		State:         db.CommentStateFetched,
	}
	if err := d.SaveComment(c); err != nil {
		t.Fatalf("SaveComment: %v", err)
	}

	got, err := d.GetComment("c1")
	if err != nil {
		t.Fatalf("GetComment: %v", err)
	}
	if got == nil {
		t.Fatal("expected comment, got nil")
	}
	if got.CommentID != c.CommentID {
		t.Errorf("CommentID = %q, want %q", got.CommentID, c.CommentID)
	}
	if got.Author != c.Author {
		t.Errorf("Author = %q, want %q", got.Author, c.Author)
	}
	if got.FilePath != c.FilePath {
		t.Errorf("FilePath = %q, want %q", got.FilePath, c.FilePath)
	}
	if got.LineNumber != c.LineNumber {
		t.Errorf("LineNumber = %d, want %d", got.LineNumber, c.LineNumber)
	}
	if got.State != c.State {
		t.Errorf("State = %q, want %q", got.State, c.State)
	}
}

func TestUpdateCommentState(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	d.SavePullRequest(&db.PullRequest{PRID: "pr1", Platform: "github", Repo: "r", BranchName: "b", LastCheckedAt: now, Status: db.PRStatusOpen})
	d.SaveComment(&db.Comment{CommentID: "c1", PRID: "pr1", Author: "a", Body: "b", CreatedAt: now, FetchedAt: now, TriageVerdict: db.VerdictPending, State: db.CommentStateFetched})

	if err := d.UpdateCommentState("c1", db.CommentStateTriaged); err != nil {
		t.Fatalf("UpdateCommentState: %v", err)
	}

	got, _ := d.GetComment("c1")
	if got.State != db.CommentStateTriaged {
		t.Errorf("State = %q, want %q", got.State, db.CommentStateTriaged)
	}
}

func TestListCommentsByPR(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	d.SavePullRequest(&db.PullRequest{PRID: "pr1", Platform: "github", Repo: "r", BranchName: "b", LastCheckedAt: now, Status: db.PRStatusOpen})
	d.SavePullRequest(&db.PullRequest{PRID: "pr2", Platform: "github", Repo: "r", BranchName: "c", LastCheckedAt: now, Status: db.PRStatusOpen})

	for i, prID := range []string{"pr1", "pr1", "pr1", "pr2", "pr2"} {
		d.SaveComment(&db.Comment{
			CommentID: fmt.Sprintf("c%d", i), PRID: prID, Author: "a", Body: "b",
			CreatedAt: now, FetchedAt: now, TriageVerdict: db.VerdictPending, State: db.CommentStateFetched,
		})
	}

	comments, err := d.ListCommentsByPR("pr1")
	if err != nil {
		t.Fatalf("ListCommentsByPR: %v", err)
	}
	if len(comments) != 3 {
		t.Errorf("got %d comments, want 3", len(comments))
	}
}

func TestListQueuedComments(t *testing.T) {
	d := newTestDB(t)
	now := truncSec(time.Now())
	d.SavePullRequest(&db.PullRequest{PRID: "pr1", Platform: "github", Repo: "r", BranchName: "b", LastCheckedAt: now, Status: db.PRStatusOpen})

	states := []string{
		db.CommentStateQueued, db.CommentStateQueued,
		db.CommentStateFetched, db.CommentStateTriaged,
		db.CommentStateParked, db.CommentStateDone,
	}
	for i, state := range states {
		d.SaveComment(&db.Comment{
			CommentID: fmt.Sprintf("c%d", i), PRID: "pr1", Author: "a", Body: "b",
			CreatedAt: now, FetchedAt: now, TriageVerdict: db.VerdictPending, State: state,
		})
	}

	comments, err := d.ListQueuedComments()
	if err != nil {
		t.Fatalf("ListQueuedComments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("got %d comments, want 2", len(comments))
	}
}
