package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/daemon"
	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/notify"
	"github.com/ahmedennaime/reviewbridge/internal/platforms"
	github_pkg "github.com/ahmedennaime/reviewbridge/internal/platforms/github"
	gitlab_pkg "github.com/ahmedennaime/reviewbridge/internal/platforms/gitlab"
	"github.com/ahmedennaime/reviewbridge/internal/poller"
	"github.com/ahmedennaime/reviewbridge/internal/queue"
	"github.com/ahmedennaime/reviewbridge/internal/triage"
)

type e2eTriager struct {
	db      *db.DB
	verdict string
}

func (m *e2eTriager) Run(comments []*db.Comment, _, _ string) ([]triage.TriageResult, error) {
	results := make([]triage.TriageResult, len(comments))
	for i, c := range comments {
		m.db.SetTriageResult(c.CommentID, m.verdict) //nolint:errcheck
		results[i] = triage.TriageResult{CommentID: c.CommentID, Verdict: m.verdict, Reason: "e2e-test"}
	}
	return results, nil
}

type wmClient struct {
	baseURL string
	http    *http.Client
}

func newWM(baseURL string) *wmClient {
	return &wmClient{baseURL: baseURL, http: &http.Client{Timeout: 5 * time.Second}}
}

func (w *wmClient) reset() error {
	req, _ := http.NewRequest("POST", w.baseURL+"/__admin/reset", nil)
	resp, err := w.http.Do(req)
	if err != nil {
		return fmt.Errorf("WireMock reset: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (w *wmClient) addMapping(mapping map[string]any) error {
	body, _ := json.Marshal(mapping)
	req, _ := http.NewRequest("POST", w.baseURL+"/__admin/mappings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return fmt.Errorf("WireMock addMapping: %w", err)
	}
	resp.Body.Close()
	return nil
}

func openDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func seedSession(t *testing.T, d *db.DB, id, branch string) {
	t.Helper()
	must(t, d.SaveSession(&db.Session{
		SessionID:    id,
		RepoPath:     "/repos/app",
		BranchName:   branch,
		LastActiveAt: time.Now(),
		Status:       db.SessionStatusActive,
	}))
}

func seedPR(t *testing.T, d *db.DB, prID, branch, sessionID string) {
	t.Helper()
	sid := sessionID
	must(t, d.SavePullRequest(&db.PullRequest{
		PRID:          prID,
		Platform:      platformFromPRID(prID),
		Repo:          repoFromPRID(prID),
		BranchName:    branch,
		SessionID:     &sid,
		LastCheckedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:        db.PRStatusOpen,
	}))
}

func seedComment(t *testing.T, d *db.DB, id, prID, state, verdict string) {
	t.Helper()
	must(t, d.SaveComment(&db.Comment{
		CommentID:     id,
		PRID:          prID,
		Author:        "reviewer",
		Body:          "Review comment " + id,
		FilePath:      "main.go",
		LineNumber:    42,
		CreatedAt:     time.Now(),
		FetchedAt:     time.Now(),
		TriageVerdict: verdict,
		State:         state,
	}))
}

func newDaemon(t *testing.T, d *db.DB, plats map[string]platforms.Platform) (*daemon.Daemon, *poller.Poller) {
	t.Helper()

	mt := &e2eTriager{db: d, verdict: db.VerdictFix}
	q := queue.New(d)
	n := notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil })
	p := poller.New(d, plats, time.Hour)

	pidPath := filepath.Join(t.TempDir(), "daemon.pid")

	dmn := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   p,
		Triage:   mt,
		Queue:    q,
		Notifier: n,
	}, pidPath)

	return dmn, p
}

func filterByState(comments []*db.Comment, state string) []*db.Comment {
	var out []*db.Comment
	for _, c := range comments {
		if c.State == state {
			out = append(out, c)
		}
	}
	return out
}

func platformFromPRID(prID string) string {
	for _, p := range []string{"github", "gitlab"} {
		if len(prID) > len(p) && prID[:len(p)] == p {
			return p
		}
	}
	return ""
}

func repoFromPRID(prID string) string {
	parts := splitN(prID, ":", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

func splitN(s, sep string, n int) []string {
	var parts []string
	for len(parts) < n-1 {
		idx := -1
		for i := 0; i <= len(s)-len(sep); i++ {
			if s[i:i+len(sep)] == sep {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+len(sep):]
	}
	parts = append(parts, s)
	return parts
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stateOrEmpty(c *db.Comment) string {
	if c == nil {
		return "<nil>"
	}
	return c.State
}

func TestE2E_GitHubHappyPath(t *testing.T) {
	ghURL := os.Getenv("REVIEWBRIDGE_GITHUB_BASE_URL")
	if ghURL == "" {
		t.Skip("REVIEWBRIDGE_GITHUB_BASE_URL not set — run via make test-e2e")
	}

	wm := newWM(ghURL)
	must(t, wm.reset())
	must(t, wm.addMapping(map[string]any{
		"request": map[string]any{
			"method":         "GET",
			"urlPathPattern": "/repos/owner/repo/pulls/1/comments",
		},
		"response": map[string]any{
			"status":  200,
			"headers": map[string]string{"Content-Type": "application/json"},
			"jsonBody": []map[string]any{
				{"id": 101, "user": map[string]string{"login": "alice"}, "body": "Missing null check", "path": "main.go", "line": 42, "created_at": "2025-06-01T10:30:00Z"},
				{"id": 102, "user": map[string]string{"login": "bob"}, "body": "Array bounds check", "path": "service.go", "line": 87, "created_at": "2025-06-01T10:31:00Z"},
			},
		},
	}))

	d := openDB(t)
	const sessionID = "gh-sess-001"
	const prID = "github:owner/repo:1"
	seedSession(t, d, sessionID, "feature/test")
	seedPR(t, d, prID, "feature/test", sessionID)

	plats := map[string]platforms.Platform{"github": github_pkg.New("test-token", ghURL)}
	dmn, _ := newDaemon(t, d, plats)

	must(t, dmn.Start())
	defer dmn.Stop()

	allComments, _ := d.ListCommentsByPR(prID)
	if len(filterByState(allComments, db.CommentStateFetched)) != 2 {
		t.Fatalf("expected 2 fetched comments, got %d", len(allComments))
	}

	dmn.ProcessOnce()

	allComments, _ = d.ListCommentsByPR(prID)
	queued := filterByState(allComments, db.CommentStateQueued)
	if len(queued) != 2 {
		t.Errorf("expected 2 queued comments after triage, got %d (states: %v)", len(queued), commentStates(allComments))
	}
}

func TestE2E_GitLabHappyPath(t *testing.T) {
	glURL := os.Getenv("REVIEWBRIDGE_GITLAB_BASE_URL")
	if glURL == "" {
		t.Skip("REVIEWBRIDGE_GITLAB_BASE_URL not set — run via make test-e2e")
	}

	wm := newWM(glURL)
	must(t, wm.reset())
	must(t, wm.addMapping(map[string]any{
		"request": map[string]any{
			"method":         "GET",
			"urlPathPattern": "/api/v4/projects/owner%2Frepo/merge_requests/7/notes",
		},
		"response": map[string]any{
			"status":  200,
			"headers": map[string]string{"Content-Type": "application/json"},
			"jsonBody": []map[string]any{
				{"id": 1, "author": map[string]string{"username": "alice"}, "body": "Missing null check", "created_at": "2025-06-01T10:30:00Z", "position": map[string]any{"new_path": "main.go", "new_line": 42}},
				{"id": 2, "author": map[string]string{"username": "bob"}, "body": "Array bounds check", "created_at": "2025-06-01T10:31:00Z", "position": map[string]any{"new_path": "service.go", "new_line": 87}},
			},
		},
	}))

	d := openDB(t)
	const sessionID = "gl-sess-001"
	const prID = "gitlab:owner/repo:7"
	seedSession(t, d, sessionID, "feature/test")
	seedPR(t, d, prID, "feature/test", sessionID)

	plats := map[string]platforms.Platform{"gitlab": gitlab_pkg.New("test-token", glURL)}
	dmn, _ := newDaemon(t, d, plats)

	must(t, dmn.Start())
	defer dmn.Stop()

	allComments, _ := d.ListCommentsByPR(prID)
	if len(filterByState(allComments, db.CommentStateFetched)) != 2 {
		t.Fatalf("expected 2 fetched comments from GitLab mock, got %d", len(allComments))
	}

	dmn.ProcessOnce()

	allComments, _ = d.ListCommentsByPR(prID)
	queued := filterByState(allComments, db.CommentStateQueued)
	if len(queued) != 2 {
		t.Errorf("expected 2 queued comments after triage, got %d", len(queued))
	}
}

func TestE2E_OfflineCatchUp(t *testing.T) {
	ghURL := os.Getenv("REVIEWBRIDGE_GITHUB_BASE_URL")
	if ghURL == "" {
		t.Skip("REVIEWBRIDGE_GITHUB_BASE_URL not set — run via make test-e2e")
	}

	wm := newWM(ghURL)
	must(t, wm.reset())
	must(t, wm.addMapping(map[string]any{
		"request": map[string]any{
			"method":         "GET",
			"urlPathPattern": "/repos/owner/repo/pulls/5/comments",
		},
		"response": map[string]any{
			"status":  200,
			"headers": map[string]string{"Content-Type": "application/json"},
			"jsonBody": []map[string]any{
				{"id": 201, "user": map[string]string{"login": "alice"}, "body": "Comment 1", "path": "a.go", "line": 1, "created_at": "2025-06-01T08:00:00Z"},
				{"id": 202, "user": map[string]string{"login": "bob"}, "body": "Comment 2", "path": "b.go", "line": 2, "created_at": "2025-06-01T08:01:00Z"},
				{"id": 203, "user": map[string]string{"login": "carol"}, "body": "Comment 3", "path": "c.go", "line": 3, "created_at": "2025-06-01T08:02:00Z"},
			},
		},
	}))

	d := openDB(t)
	const sessionID = "catchup-sess"
	const prID = "github:owner/repo:5"
	seedSession(t, d, sessionID, "feature/catchup")
	seedPR(t, d, prID, "feature/catchup", sessionID)

	plats := map[string]platforms.Platform{"github": github_pkg.New("test-token", ghURL)}
	dmn, _ := newDaemon(t, d, plats)

	must(t, dmn.Start())
	defer dmn.Stop()

	allComments, _ := d.ListCommentsByPR(prID)
	fetched := filterByState(allComments, db.CommentStateFetched)
	if len(fetched) != 3 {
		t.Errorf("expected 3 comments fetched on startup catch-up, got %d", len(fetched))
	}
}

func TestE2E_DaemonRestartNoCommentsLost(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "reviewbridge.db")

	d1, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	sessionID := "restart-sess"
	prID := "github:owner/repo:99"
	sid := sessionID
	must(t, d1.SaveSession(&db.Session{
		SessionID:    sessionID,
		RepoPath:     "/repos/app",
		BranchName:   "feature/restart",
		LastActiveAt: time.Now(),
		Status:       db.SessionStatusActive,
	}))
	must(t, d1.SavePullRequest(&db.PullRequest{
		PRID: prID, Platform: "github", Repo: "owner/repo",
		BranchName: "feature/restart", SessionID: &sid,
		LastCheckedAt: time.Now(), Status: db.PRStatusOpen,
	}))
	seedComment(t, d1, "rc1", prID, db.CommentStateQueued, db.VerdictFix)
	seedComment(t, d1, "rc2", prID, db.CommentStateQueued, db.VerdictFix)
	seedComment(t, d1, "rc3", prID, db.CommentStateQueued, db.VerdictFix)
	d1.Close()

	d2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer d2.Close()

	allComments, _ := d2.ListCommentsByPR(prID)
	queued := filterByState(allComments, db.CommentStateQueued)
	if len(queued) != 3 {
		t.Errorf("after restart expected 3 queued comments, got %d (total %d)", len(queued), len(allComments))
	}
}

func TestE2E_SkipVerdictNotQueued(t *testing.T) {
	d := openDB(t)

	seedSession(t, d, "sess-a", "feature/a")
	seedPR(t, d, "github:owner/repo:10", "feature/a", "sess-a")
	seedComment(t, d, "c1", "github:owner/repo:10", db.CommentStateFetched, db.VerdictPending)

	mt := &e2eTriager{db: d, verdict: db.VerdictSkip}
	q := queue.New(d)
	n := notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil })
	pidPath := filepath.Join(t.TempDir(), "daemon.pid")

	dmn := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   nil,
		Triage:   mt,
		Queue:    q,
		Notifier: n,
	}, pidPath)

	dmn.ProcessOnce()

	c, _ := d.GetComment("c1")
	if c == nil {
		t.Fatal("comment not found")
	}
	if c.State == db.CommentStateQueued {
		t.Error("skip-verdict comment should not be queued")
	}
	_ = stateOrEmpty(c)
}

func commentStates(comments []*db.Comment) []string {
	states := make([]string, len(comments))
	for i, c := range comments {
		states[i] = c.CommentID + "=" + c.State
	}
	return states
}
