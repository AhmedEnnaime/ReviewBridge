package integration_test

import (
	"os"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/platforms/github"
	"github.com/ahmedennaime/reviewbridge/internal/platforms/gitlab"
)

func githubBaseURL() string {
	if u := os.Getenv("REVIEWBRIDGE_GITHUB_BASE_URL"); u != "" {
		return u
	}
	return ""
}

func gitlabBaseURL() string {
	if u := os.Getenv("REVIEWBRIDGE_GITLAB_BASE_URL"); u != "" {
		return u
	}
	return ""
}

func skipIfNoMock(t *testing.T, baseURL string) {
	t.Helper()
	if baseURL == "" {
		t.Skip("set REVIEWBRIDGE_GITHUB_BASE_URL or REVIEWBRIDGE_GITLAB_BASE_URL to run integration tests")
	}
}

func TestGitHubIntegration_ListPRs(t *testing.T) {
	base := githubBaseURL()
	skipIfNoMock(t, base)
	client := github.New("test-token", base)

	prs, err := client.ListOpenPullRequests("owner/repo")
	if err != nil {
		t.Fatalf("ListOpenPullRequests: %v", err)
	}
	if len(prs) == 0 {
		t.Error("expected at least one PR from mock")
	}
	for _, pr := range prs {
		if pr.Number == 0 {
			t.Error("PR has zero number")
		}
		if pr.SourceBranch == "" {
			t.Error("PR has empty source branch")
		}
	}
}

func TestGitHubIntegration_CommentsWithPagination(t *testing.T) {
	base := githubBaseURL()
	skipIfNoMock(t, base)
	client := github.New("test-token", base)

	since := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	comments, err := client.ListCommentsSince("owner/repo", 12, since)
	if err != nil {
		t.Fatalf("ListCommentsSince: %v", err)
	}
	if len(comments) == 0 {
		t.Error("expected at least one comment from mock")
	}
	for _, c := range comments {
		if c.ID == "" {
			t.Error("comment has empty ID")
		}
		if c.Body == "" {
			t.Error("comment has empty body")
		}
	}
}

func TestGitLabIntegration_ListMRs(t *testing.T) {
	base := gitlabBaseURL()
	skipIfNoMock(t, base)
	client := gitlab.New("test-token", base)

	mrs, err := client.ListOpenPullRequests("owner/repo")
	if err != nil {
		t.Fatalf("ListOpenPullRequests: %v", err)
	}
	if len(mrs) == 0 {
		t.Error("expected at least one MR from mock")
	}
	for _, mr := range mrs {
		if mr.Number == 0 {
			t.Error("MR has zero IID")
		}
		if mr.SourceBranch == "" {
			t.Error("MR has empty source branch")
		}
	}
}

func TestGitLabIntegration_NotesWithPagination(t *testing.T) {
	base := gitlabBaseURL()
	skipIfNoMock(t, base)
	client := gitlab.New("test-token", base)

	since := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	notes, err := client.ListCommentsSince("owner/repo", 7, since)
	if err != nil {
		t.Fatalf("ListCommentsSince: %v", err)
	}
	if len(notes) == 0 {
		t.Error("expected at least one note from mock")
	}
}

func TestGitLabIntegration_SelfHosted(t *testing.T) {
	base := gitlabBaseURL()
	skipIfNoMock(t, base)
	client := gitlab.New("test-token", base)

	_, err := client.ListOpenPullRequests("owner/repo")
	if err != nil {
		t.Fatalf("self-hosted request failed: %v", err)
	}
}
