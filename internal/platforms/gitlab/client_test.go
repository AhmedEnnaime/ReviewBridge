package gitlab_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/platforms"
	"github.com/ahmedennaime/reviewbridge/internal/platforms/gitlab"
)

func newMockServer(handler http.Handler) (*httptest.Server, *gitlab.Client) {
	srv := httptest.NewServer(handler)
	return srv, gitlab.New("test-token", srv.URL)
}

func TestGitLabListOpenMRs(t *testing.T) {
	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"iid":7,"title":"Fix auth","state":"opened","source_branch":"feature/issue-a","web_url":"https://gitlab.com/owner/repo/-/merge_requests/7"},
			{"iid":8,"title":"Add tests","state":"opened","source_branch":"feature/issue-b","web_url":"https://gitlab.com/owner/repo/-/merge_requests/8"}
		]`)
	}))
	defer srv.Close()

	prs, err := client.ListOpenPullRequests("owner/repo")
	if err != nil {
		t.Fatalf("ListOpenPullRequests: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d MRs, want 2", len(prs))
	}
	if prs[0].Number != 7 {
		t.Errorf("Number = %d, want 7", prs[0].Number)
	}
	if prs[0].SourceBranch != "feature/issue-a" {
		t.Errorf("SourceBranch = %q, want feature/issue-a", prs[0].SourceBranch)
	}
}

func TestGitLabListCommentsSince(t *testing.T) {
	since := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"id":1,"author":{"username":"alice"},"body":"old note","created_at":"2024-01-14T10:00:00Z"},
			{"id":2,"author":{"username":"alice"},"body":"also old","created_at":"2024-01-15T10:00:00Z"},
			{"id":3,"author":{"username":"bob"},"body":"new note 1","created_at":"2024-01-15T13:00:00Z","position":{"new_path":"main.go","new_line":42}},
			{"id":4,"author":{"username":"bob"},"body":"new note 2","created_at":"2024-01-16T09:00:00Z"},
			{"id":5,"author":{"username":"bob"},"body":"new note 3","created_at":"2024-01-17T09:00:00Z"}
		]`)
	}))
	defer srv.Close()

	comments, err := client.ListCommentsSince("owner/repo", 7, since)
	if err != nil {
		t.Fatalf("ListCommentsSince: %v", err)
	}
	if len(comments) != 3 {
		t.Errorf("got %d comments, want 3", len(comments))
	}
	if comments[0].FilePath != "main.go" {
		t.Errorf("FilePath = %q, want main.go", comments[0].FilePath)
	}
	if comments[0].Line != 42 {
		t.Errorf("Line = %d, want 42", comments[0].Line)
	}
}

func TestGitLabGetDiff(t *testing.T) {
	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"changes":[
			{"new_path":"main.go","diff":"@@ -1,3 +1,4 @@\n context\n+added line\n context\n"},
			{"new_path":"util.go","diff":"@@ -5,3 +5,3 @@\n context\n-old line\n+new line\n"}
		]}`)
	}))
	defer srv.Close()

	diff, err := client.GetDiff("owner/repo", 7)
	if err != nil {
		t.Fatalf("GetDiff: %v", err)
	}
	if diff == "" {
		t.Error("expected non-empty diff")
	}
	if !contains(diff, "main.go") || !contains(diff, "util.go") {
		t.Errorf("diff missing expected file names:\n%s", diff)
	}
}

func TestGitLabUnauthorized(t *testing.T) {
	srv, client := newMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := client.ListOpenPullRequests("owner/repo")
	if !errors.Is(err, platforms.ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestGitLabSelfHostedURL(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	client := gitlab.New("test-token", srv.URL)
	client.ListOpenPullRequests("owner/repo")

	wantHost := fmt.Sprintf("127.0.0.1:%s", srv.URL[len("http://127.0.0.1:"):])
	if gotHost != wantHost {
		t.Errorf("request went to %q, want %q", gotHost, wantHost)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
