package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

func TestQueueShowsGroupedByPR(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	saveQueuedComment(t, database, "c1", "github:owner/repo:1", "file.go", 10, db.VerdictFix)
	saveQueuedComment(t, database, "c2", "github:owner/repo:1", "file.go", 20, db.VerdictFix)
	saveQueuedComment(t, database, "c3", "github:owner/repo:2", "main.go", 5, db.VerdictYourCall)
	saveQueuedComment(t, database, "c4", "github:owner/repo:2", "main.go", 8, db.VerdictSkip)

	var out strings.Builder
	if err := runQueue(&out, database); err != nil {
		t.Fatalf("runQueue: %v", err)
	}

	result := out.String()
	if strings.Count(result, "PR #") != 2 {
		t.Errorf("expected 2 PR groups, output:\n%s", result)
	}
}

func TestQueueShowsTriageVerdicts(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	saveQueuedComment(t, database, "v1", "github:owner/repo:1", "a.go", 1, db.VerdictFix)
	saveQueuedComment(t, database, "v2", "github:owner/repo:1", "b.go", 2, db.VerdictYourCall)
	saveQueuedComment(t, database, "v3", "github:owner/repo:1", "c.go", 3, db.VerdictSkip)

	var out strings.Builder
	if err := runQueue(&out, database); err != nil {
		t.Fatalf("runQueue: %v", err)
	}

	result := out.String()
	if !strings.Contains(result, "✅") {
		t.Error("output missing fix icon ✅")
	}
	if !strings.Contains(result, "⚠️") {
		t.Error("output missing your-call icon ⚠️")
	}
	if !strings.Contains(result, "❌") {
		t.Error("output missing skip icon ❌")
	}
}

func TestQueueEmptyState(t *testing.T) {
	database, cleanup := openTestDB(t)
	defer cleanup()

	var out strings.Builder
	if err := runQueue(&out, database); err != nil {
		t.Fatalf("runQueue: %v", err)
	}

	if !strings.Contains(strings.ToLower(out.String()), "queue is empty") {
		t.Errorf("expected empty queue message, got: %q", out.String())
	}
}

func saveQueuedComment(t *testing.T, database *db.DB, id, prID, file string, line int, verdict string) {
	t.Helper()
	must(t, database.SaveComment(&db.Comment{
		CommentID:     id,
		PRID:          prID,
		Author:        "reviewer",
		Body:          "Some comment body for " + id,
		FilePath:      file,
		LineNumber:    line,
		CreatedAt:     time.Now(),
		FetchedAt:     time.Now(),
		TriageVerdict: verdict,
		State:         db.CommentStateQueued,
	}))
}
