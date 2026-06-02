package triage_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/triage"
)

func claudeResponse(verdicts []map[string]string) string {
	body, _ := json.Marshal(map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": verdictsJSON(verdicts)},
		},
	})
	return string(body)
}

func verdictsJSON(verdicts []map[string]string) string {
	b, _ := json.Marshal(verdicts)
	return string(b)
}

func newTriageEnv(t *testing.T, handler http.Handler) (*db.DB, *triage.Engine) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	e := triage.New("test-key", d).WithEndpoint(srv.URL)
	return d, e
}

func seedComment(t *testing.T, d *db.DB, id, prID string) {
	t.Helper()
	d.SavePullRequest(&db.PullRequest{
		PRID: prID, Platform: "github", Repo: "owner/repo",
		BranchName: "feature/a", LastCheckedAt: time.Now(), Status: db.PRStatusOpen,
	})
	d.SaveComment(&db.Comment{
		CommentID: id, PRID: prID, Author: "alice", Body: "fix this",
		CreatedAt: time.Now(), FetchedAt: time.Now(),
		TriageVerdict: db.VerdictPending, State: db.CommentStateFetched,
	})
}

func TestParserExtractsAllVerdicts(t *testing.T) {
	verdicts := []map[string]string{
		{"comment_id": "c1", "verdict": "fix", "reason": "real bug"},
		{"comment_id": "c2", "verdict": "skip", "reason": "style nit"},
		{"comment_id": "c3", "verdict": "your-call", "reason": "ambiguous"},
		{"comment_id": "c4", "verdict": "fix", "reason": "security issue"},
	}
	d, e := newTriageEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, claudeResponse(verdicts))
	}))

	for i := 1; i <= 4; i++ {
		seedComment(t, d, fmt.Sprintf("c%d", i), "github:owner/repo:12")
	}
	comments := []*db.Comment{
		{CommentID: "c1"}, {CommentID: "c2"}, {CommentID: "c3"}, {CommentID: "c4"},
	}

	results, err := e.Run(comments, "", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 4 {
		t.Errorf("got %d results, want 4", len(results))
	}
}

func TestParserHandlesFixVerdict(t *testing.T) {
	d, e := newTriageEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, claudeResponse([]map[string]string{
			{"comment_id": "c1", "verdict": "fix", "reason": "real bug"},
		}))
	}))
	seedComment(t, d, "c1", "github:owner/repo:12")
	results, err := e.Run([]*db.Comment{{CommentID: "c1"}}, "", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results[0].Verdict != db.VerdictFix {
		t.Errorf("verdict = %q, want %q", results[0].Verdict, db.VerdictFix)
	}
}

func TestParserHandlesYourCallVerdict(t *testing.T) {
	d, e := newTriageEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, claudeResponse([]map[string]string{
			{"comment_id": "c1", "verdict": "your-call", "reason": "ambiguous"},
		}))
	}))
	seedComment(t, d, "c1", "github:owner/repo:12")
	results, err := e.Run([]*db.Comment{{CommentID: "c1"}}, "", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results[0].Verdict != db.VerdictYourCall {
		t.Errorf("verdict = %q, want %q", results[0].Verdict, db.VerdictYourCall)
	}
}

func TestParserHandlesSkipVerdict(t *testing.T) {
	d, e := newTriageEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, claudeResponse([]map[string]string{
			{"comment_id": "c1", "verdict": "skip", "reason": "style nit"},
		}))
	}))
	seedComment(t, d, "c1", "github:owner/repo:12")
	results, err := e.Run([]*db.Comment{{CommentID: "c1"}}, "", "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results[0].Verdict != db.VerdictSkip {
		t.Errorf("verdict = %q, want %q", results[0].Verdict, db.VerdictSkip)
	}
}

func TestParserInvalidJSON(t *testing.T) {
	_, e := newTriageEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"content":[{"type":"text","text":"not valid json at all!!!"}]}`)
	}))
	_, err := e.Run([]*db.Comment{{CommentID: "c1"}}, "", "")
	if err == nil {
		t.Error("expected error for invalid JSON response, got nil")
	}
}

func TestParserMissingCommentID(t *testing.T) {
	_, e := newTriageEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"content":[{"type":"text","text":"[{\"verdict\":\"fix\",\"reason\":\"bug\"}]"}]}`)
	}))
	_, err := e.Run([]*db.Comment{{CommentID: "c1"}}, "", "")
	if err == nil {
		t.Error("expected error for missing comment_id, got nil")
	}
}

func TestTriageUpdatesCommentStates(t *testing.T) {
	d, e := newTriageEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, claudeResponse([]map[string]string{
			{"comment_id": "c1", "verdict": "fix", "reason": "bug"},
			{"comment_id": "c2", "verdict": "skip", "reason": "nit"},
		}))
	}))
	seedComment(t, d, "c1", "github:owner/repo:12")
	seedComment(t, d, "c2", "github:owner/repo:12")

	comments := []*db.Comment{{CommentID: "c1"}, {CommentID: "c2"}}
	if _, err := e.Run(comments, "", ""); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, id := range []string{"c1", "c2"} {
		c, _ := d.GetComment(id)
		if c.State != db.CommentStateTriaged {
			t.Errorf("comment %s state = %q, want %q", id, c.State, db.CommentStateTriaged)
		}
		if c.TriageVerdict == db.VerdictPending {
			t.Errorf("comment %s verdict still pending after triage", id)
		}
	}
}

func TestTriageHandlesAPIError(t *testing.T) {
	d, e := newTriageEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"internal error"}}`)
	}))
	seedComment(t, d, "c1", "github:owner/repo:12")

	_, err := e.Run([]*db.Comment{{CommentID: "c1"}}, "", "")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}

	c, _ := d.GetComment("c1")
	if c.State != db.CommentStateFetched {
		t.Errorf("comment state changed to %q on API error, want %q", c.State, db.CommentStateFetched)
	}
}
