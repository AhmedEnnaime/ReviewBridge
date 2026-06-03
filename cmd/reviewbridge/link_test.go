package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

func TestLinkUpdatesSessionBranch(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	must(t, database.SaveSession(&db.Session{
		SessionID:    "sess001",
		RepoPath:     "/repos/app",
		BranchName:   "main",
		LastActiveAt: time.Now(),
		Status:       db.SessionStatusActive,
	}))

	var out strings.Builder
	if err := runLink(&out, database, "sess001", "feature/new-branch", ""); err != nil {
		t.Fatalf("runLink: %v", err)
	}

	s, _ := database.GetSession("sess001")
	if s.BranchName != "feature/new-branch" {
		t.Errorf("expected branch feature/new-branch, got %s", s.BranchName)
	}
	if !strings.Contains(out.String(), "feature/new-branch") {
		t.Error("output should mention the branch name")
	}
}

func TestLinkSessionNotFound(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	err := runLink(&strings.Builder{}, database, "nonexistent", "feature/x", "")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestLinkRequiresSession(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	err := runLink(&strings.Builder{}, database, "", "feature/x", "")
	if err == nil || !strings.Contains(err.Error(), "--session") {
		t.Errorf("expected --session error, got: %v", err)
	}
}

func TestLinkLinksPRToSession(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	sessionID := "sess002"
	must(t, database.SaveSession(&db.Session{
		SessionID:    sessionID,
		RepoPath:     "/repos/app",
		BranchName:   "feature/x",
		LastActiveAt: time.Now(),
		Status:       db.SessionStatusActive,
	}))
	must(t, database.SavePullRequest(&db.PullRequest{
		PRID:          "github:owner/repo:7",
		Platform:      "github",
		Repo:          "owner/repo",
		BranchName:    "feature/x",
		LastCheckedAt: time.Now(),
		Status:        db.PRStatusOpen,
	}))

	var out strings.Builder
	if err := runLink(&out, database, sessionID, "", "github:owner/repo:7"); err != nil {
		t.Fatalf("runLink: %v", err)
	}

	pr, _ := database.GetPullRequest("github:owner/repo:7")
	if pr.SessionID == nil || *pr.SessionID != sessionID {
		t.Error("PR should be linked to session")
	}
}
