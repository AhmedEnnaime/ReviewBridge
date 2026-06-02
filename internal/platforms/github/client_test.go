package github_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/platforms"
	"github.com/ahmedennaime/reviewbridge/internal/platforms/github"
)

func newMockServer(handler http.Handler) (*httptest.Server, *github.Client) {
	srv := httptest.NewServer(handler)
	return srv, github.New("test-token", srv.URL)
}

func TestGitHubListOpenPRs(t *testing.T) {
	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"number":12,"title":"Fix auth","state":"open","head":{"ref":"feature/issue-a"},"html_url":"https://github.com/owner/repo/pull/12"},
			{"number":13,"title":"Add tests","state":"open","head":{"ref":"feature/issue-b"},"html_url":"https://github.com/owner/repo/pull/13"}
		]`)
	}))
	defer srv.Close()

	prs, err := client.ListOpenPullRequests("owner/repo")
	if err != nil {
		t.Fatalf("ListOpenPullRequests: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d PRs, want 2", len(prs))
	}
	if prs[0].Number != 12 {
		t.Errorf("Number = %d, want 12", prs[0].Number)
	}
	if prs[0].SourceBranch != "feature/issue-a" {
		t.Errorf("SourceBranch = %q, want feature/issue-a", prs[0].SourceBranch)
	}
}

func TestGitHubListCommentsSince(t *testing.T) {
	since := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	line1, line2, line3 := 10, 20, 30

	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[
			{"id":1,"user":{"login":"alice"},"body":"old comment","path":"a.go","line":%d,"created_at":"2024-01-14T10:00:00Z"},
			{"id":2,"user":{"login":"alice"},"body":"also old","path":"b.go","line":%d,"created_at":"2024-01-15T10:00:00Z"},
			{"id":3,"user":{"login":"bob"},"body":"new comment 1","path":"c.go","line":%d,"created_at":"2024-01-15T13:00:00Z"},
			{"id":4,"user":{"login":"bob"},"body":"new comment 2","path":"d.go","line":40,"created_at":"2024-01-16T09:00:00Z"},
			{"id":5,"user":{"login":"bob"},"body":"new comment 3","path":"e.go","line":50,"created_at":"2024-01-17T09:00:00Z"}
		]`, line1, line2, line3)
	}))
	defer srv.Close()

	comments, err := client.ListCommentsSince("owner/repo", 12, since)
	if err != nil {
		t.Fatalf("ListCommentsSince: %v", err)
	}
	if len(comments) != 3 {
		t.Errorf("got %d comments, want 3", len(comments))
	}
}

func TestGitHubGetDiff(t *testing.T) {
	wantDiff := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n"
	srv, _ := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, wantDiff)
	}))
	defer srv.Close()

	client := github.New("test-token", srv.URL)
	diff, err := client.GetDiff("owner/repo", 12)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	if diff != wantDiff {
		t.Errorf("diff mismatch\ngot:  %q\nwant: %q", diff, wantDiff)
	}
}

func TestGitHubUnauthorized(t *testing.T) {
	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := client.ListOpenPullRequests("owner/repo")
	if !errors.Is(err, platforms.ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestGitHubRateLimited(t *testing.T) {
	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := client.ListOpenPullRequests("owner/repo")
	if !errors.Is(err, platforms.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
}

func TestGitHubRepoNotFound(t *testing.T) {
	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := client.GetPullRequest("owner/repo", 99)
	if !errors.Is(err, platforms.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}
