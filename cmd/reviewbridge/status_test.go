package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

func TestStatusShowsAllSessions(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	for _, id := range []string{"aaa111", "bbb222", "ccc333"} {
		must(t, database.SaveSession(&db.Session{
			SessionID:    id,
			RepoPath:     "/repos/app",
			BranchName:   "feature/" + id,
			LastActiveAt: time.Now(),
			Status:       db.SessionStatusActive,
		}))
	}

	var out strings.Builder
	if err := runStatus(&out, database); err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	result := out.String()
	for _, id := range []string{"aaa111", "bbb222", "ccc333"} {
		if !strings.Contains(result, id) {
			t.Errorf("output missing session %s", id)
		}
	}
}

func TestStatusShowsLinkedPRs(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	sessionID := "sess001"
	must(t, database.SaveSession(&db.Session{
		SessionID:    sessionID,
		RepoPath:     "/repos/app",
		BranchName:   "feature/x",
		LastActiveAt: time.Now(),
		Status:       db.SessionStatusActive,
	}))
	must(t, database.SavePullRequest(&db.PullRequest{
		PRID:          "github:owner/repo:42",
		Platform:      "github",
		Repo:          "owner/repo",
		BranchName:    "feature/x",
		SessionID:     &sessionID,
		LastCheckedAt: time.Now(),
		Status:        db.PRStatusOpen,
	}))

	var out strings.Builder
	if err := runStatus(&out, database); err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	result := out.String()
	if !strings.Contains(result, "42") {
		t.Error("output should contain PR number 42")
	}
	if !strings.Contains(result, "github") {
		t.Error("output should contain platform name")
	}
}

func TestStatusEmptyState(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	var out strings.Builder
	if err := runStatus(&out, database); err != nil {
		t.Fatalf("runStatus: %v", err)
	}

	if !strings.Contains(strings.ToLower(out.String()), "no sessions") {
		t.Errorf("expected friendly empty message, got: %q", out.String())
	}
}

func openTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	return d, func() { d.Close() }
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
