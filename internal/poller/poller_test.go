package poller_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/platforms"
	"github.com/ahmedennaime/reviewbridge/internal/poller"
)

type mockPlatform struct {
	openPRs   []*platforms.PullRequest
	comments  []*platforms.Comment
	listCount int64
	returnErr error
}

func (m *mockPlatform) ListOpenPullRequests(_ string) ([]*platforms.PullRequest, error) {
	return m.openPRs, m.returnErr
}

func (m *mockPlatform) GetPullRequest(_ string, _ int) (*platforms.PullRequest, error) {
	return nil, nil
}

func (m *mockPlatform) ListCommentsSince(_ string, _ int, _ time.Time) ([]*platforms.Comment, error) {
	atomic.AddInt64(&m.listCount, 1)
	return m.comments, m.returnErr
}

func (m *mockPlatform) GetDiff(_ string, _ int) (string, error) {
	return "", nil
}

func newTestEnv(t *testing.T) (*db.DB, *poller.Poller, *mockPlatform) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	mock := &mockPlatform{}
	p := poller.New(d, map[string]platforms.Platform{"github": mock}, time.Hour)
	return d, p, mock
}

func seedPR(t *testing.T, d *db.DB, prid, branch string, since time.Time) {
	t.Helper()
	d.SavePullRequest(&db.PullRequest{
		PRID:          prid,
		Platform:      "github",
		Repo:          "owner/repo",
		BranchName:    branch,
		LastCheckedAt: since,
		Status:        db.PRStatusOpen,
	})
}

func TestPollerFetchesNewComments(t *testing.T) {
	d, p, mock := newTestEnv(t)
	seedPR(t, d, "github:owner/repo:12", "feature/a", time.Now().Add(-time.Hour))
	mock.comments = []*platforms.Comment{
		{ID: "1", Author: "alice", Body: "fix this", CreatedAt: time.Now()},
		{ID: "2", Author: "bob", Body: "and this", CreatedAt: time.Now()},
		{ID: "3", Author: "carol", Body: "also this", CreatedAt: time.Now()},
	}

	p.Poll()

	comments, err := d.ListCommentsByPR("github:owner/repo:12")
	if err != nil {
		t.Fatalf("ListCommentsByPR: %v", err)
	}
	if len(comments) != 3 {
		t.Errorf("got %d comments, want 3", len(comments))
	}
	for _, c := range comments {
		if c.State != db.CommentStateFetched {
			t.Errorf("comment state = %q, want %q", c.State, db.CommentStateFetched)
		}
	}
}

func TestPollerUpdatesLastChecked(t *testing.T) {
	d, p, _ := newTestEnv(t)
	old := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	seedPR(t, d, "github:owner/repo:12", "feature/a", old)

	before := time.Now().Add(-time.Second)
	p.Poll()

	pr, _ := d.GetPullRequest("github:owner/repo:12")
	if !pr.LastCheckedAt.After(before) {
		t.Errorf("LastCheckedAt not updated: got %v, want after %v", pr.LastCheckedAt, before)
	}
}

func TestPollerSkipsClosedPRs(t *testing.T) {
	d, p, mock := newTestEnv(t)
	d.SavePullRequest(&db.PullRequest{
		PRID:          "github:owner/repo:12",
		Platform:      "github",
		Repo:          "owner/repo",
		BranchName:    "feature/a",
		LastCheckedAt: time.Now().Add(-time.Hour),
		Status:        db.PRStatusMerged,
	})
	d.SavePullRequest(&db.PullRequest{
		PRID:          "github:owner/repo:13",
		Platform:      "github",
		Repo:          "owner/repo",
		BranchName:    "feature/b",
		LastCheckedAt: time.Now().Add(-time.Hour),
		Status:        db.PRStatusClosed,
	})

	p.Poll()

	if atomic.LoadInt64(&mock.listCount) != 0 {
		t.Errorf("platform called %d times, want 0 (all PRs are closed/merged)", mock.listCount)
	}
}

func TestPollerDeduplicatesComments(t *testing.T) {
	d, p, mock := newTestEnv(t)
	seedPR(t, d, "github:owner/repo:12", "feature/a", time.Now().Add(-time.Hour))

	comment := &platforms.Comment{ID: "1", Author: "alice", Body: "fix this", CreatedAt: time.Now()}
	mock.comments = []*platforms.Comment{comment}

	p.Poll()
	p.Poll()

	comments, _ := d.ListCommentsByPR("github:owner/repo:12")
	if len(comments) != 1 {
		t.Errorf("got %d comments after two polls, want 1 (no duplicates)", len(comments))
	}
}

func TestPollerHandlesPlatformError(t *testing.T) {
	d, p, mock := newTestEnv(t)
	seedPR(t, d, "github:owner/repo:12", "feature/a", time.Now().Add(-time.Hour))
	seedPR(t, d, "github:owner/repo:13", "feature/b", time.Now().Add(-time.Hour))
	mock.returnErr = errors.New("API unavailable")

	p.Poll()

	comments, _ := d.ListCommentsByPR("github:owner/repo:12")
	if len(comments) != 0 {
		t.Errorf("expected 0 comments on error, got %d", len(comments))
	}
}

func TestPollerRespectsPollInterval(t *testing.T) {
	d, _, mock := newTestEnv(t)
	seedPR(t, d, "github:owner/repo:12", "feature/a", time.Now().Add(-time.Hour))

	fakeTick := make(chan time.Time)
	p := poller.New(d, map[string]platforms.Platform{"github": mock}, time.Hour).
		WithTickerFn(func(_ time.Duration) (<-chan time.Time, func()) {
			return fakeTick, func() {}
		})

	p.Start()
	time.Sleep(100 * time.Millisecond)

	countAfterStart := atomic.LoadInt64(&mock.listCount)
	if countAfterStart != 1 {
		t.Errorf("listCount after start = %d, want 1 (only CatchUp)", countAfterStart)
	}

	fakeTick <- time.Now()
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt64(&mock.listCount) != 2 {
		t.Errorf("listCount after tick = %d, want 2", mock.listCount)
	}
	p.Stop()
}

func TestStartupCatchUpFetchesAllMissedComments(t *testing.T) {
	d, p, mock := newTestEnv(t)
	old := time.Now().Add(-6 * time.Hour)
	seedPR(t, d, "github:owner/repo:12", "feature/a", old)
	mock.comments = []*platforms.Comment{
		{ID: "1", Author: "alice", Body: "c1", CreatedAt: old.Add(time.Hour)},
		{ID: "2", Author: "alice", Body: "c2", CreatedAt: old.Add(2 * time.Hour)},
		{ID: "3", Author: "bob", Body: "c3", CreatedAt: old.Add(3 * time.Hour)},
		{ID: "4", Author: "bob", Body: "c4", CreatedAt: old.Add(4 * time.Hour)},
	}

	p.CatchUp()

	comments, _ := d.ListCommentsByPR("github:owner/repo:12")
	if len(comments) != 4 {
		t.Errorf("got %d comments after CatchUp, want 4", len(comments))
	}
}

func TestStartupCatchUpRunsBeforeFirstInterval(t *testing.T) {
	d, _, mock := newTestEnv(t)
	seedPR(t, d, "github:owner/repo:12", "feature/a", time.Now().Add(-time.Hour))

	neverTick := make(chan time.Time)
	p := poller.New(d, map[string]platforms.Platform{"github": mock}, time.Hour).
		WithTickerFn(func(_ time.Duration) (<-chan time.Time, func()) {
			return neverTick, func() {}
		})

	p.Start()

	if atomic.LoadInt64(&mock.listCount) != 1 {
		t.Errorf("listCount = %d, want 1 (CatchUp ran before any tick)", mock.listCount)
	}
	p.Stop()
}

func TestDiscoversPRForNewBranch(t *testing.T) {
	d, p, mock := newTestEnv(t)
	mock.openPRs = []*platforms.PullRequest{
		{Number: 12, Title: "Fix auth", SourceBranch: "feature/issue-a", State: "open"},
		{Number: 13, Title: "Other", SourceBranch: "feature/other", State: "open"},
	}

	session := &db.Session{
		SessionID:  "abc123",
		RepoPath:   "/repos/myapp",
		BranchName: "feature/issue-a",
		Status:     db.SessionStatusActive,
	}
	d.SaveSession(session)

	if err := p.DiscoverPRs(session, "github", "owner/repo"); err != nil {
		t.Fatalf("DiscoverPRs: %v", err)
	}

	pr, err := d.GetPullRequest("github:owner/repo:12")
	if err != nil || pr == nil {
		t.Fatalf("PR not found in DB: %v", err)
	}
	if pr.SessionID == nil || *pr.SessionID != "abc123" {
		t.Errorf("SessionID = %v, want abc123", pr.SessionID)
	}
	if pr.BranchName != "feature/issue-a" {
		t.Errorf("BranchName = %q, want feature/issue-a", pr.BranchName)
	}

	pr13, _ := d.GetPullRequest("github:owner/repo:13")
	if pr13 != nil {
		t.Error("PR #13 should not be registered (different branch)")
	}
}

func TestNoPRForBranch(t *testing.T) {
	d, p, mock := newTestEnv(t)
	mock.openPRs = []*platforms.PullRequest{
		{Number: 12, SourceBranch: "feature/other", State: "open"},
	}
	session := &db.Session{
		SessionID:  "abc123",
		RepoPath:   "/repos/myapp",
		BranchName: "feature/no-pr",
		Status:     db.SessionStatusActive,
	}
	d.SaveSession(session)

	if err := p.DiscoverPRs(session, "github", "owner/repo"); err != nil {
		t.Fatalf("DiscoverPRs: %v", err)
	}

	prs, _ := d.ListOpenPullRequests()
	if len(prs) != 0 {
		t.Errorf("got %d PRs, want 0", len(prs))
	}
}

func TestMultiplePRsForBranch(t *testing.T) {
	d, p, mock := newTestEnv(t)
	mock.openPRs = []*platforms.PullRequest{
		{Number: 12, SourceBranch: "feature/issue-a", State: "open"},
		{Number: 13, SourceBranch: "feature/issue-a", State: "open"},
	}
	session := &db.Session{
		SessionID:  "abc123",
		RepoPath:   "/repos/myapp",
		BranchName: "feature/issue-a",
		Status:     db.SessionStatusActive,
	}
	d.SaveSession(session)

	if err := p.DiscoverPRs(session, "github", "owner/repo"); err != nil {
		t.Fatalf("DiscoverPRs: %v", err)
	}

	prs, _ := d.ListOpenPullRequests()
	if len(prs) != 2 {
		t.Errorf("got %d PRs, want 2", len(prs))
	}
}
