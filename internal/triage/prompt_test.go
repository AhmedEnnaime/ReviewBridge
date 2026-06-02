package triage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/triage"
)

func makeComment(id, author, body, file string, line int) *db.Comment {
	return &db.Comment{
		CommentID: id,
		Author:    author,
		Body:      body,
		FilePath:  file,
		LineNumber: line,
		CreatedAt: time.Now(),
		FetchedAt: time.Now(),
	}
}

func TestPromptIncludesAllComments(t *testing.T) {
	comments := []*db.Comment{
		makeComment("c1", "alice", "missing null check", "main.go", 10),
		makeComment("c2", "bob", "array out of bounds", "util.go", 20),
		makeComment("c3", "carol", "unused variable", "service.go", 30),
		makeComment("c4", "dave", "consider extracting", "handler.go", 40),
	}

	prompt := triage.BuildPrompt("diff content", comments, "")

	for _, c := range comments {
		if !strings.Contains(prompt, c.Body) {
			t.Errorf("prompt missing comment body: %q", c.Body)
		}
		if !strings.Contains(prompt, c.CommentID) {
			t.Errorf("prompt missing comment ID: %q", c.CommentID)
		}
	}
}

func TestPromptIncludesCLAUDEMD(t *testing.T) {
	dir := t.TempDir()
	content := "## Project conventions\nUse handler → service → repo pattern."
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0600)

	prompt := triage.BuildPrompt("", []*db.Comment{makeComment("c1", "a", "b", "f.go", 1)}, dir)

	if !strings.Contains(prompt, content) {
		t.Error("prompt does not contain CLAUDE.md content")
	}
}

func TestPromptWithoutCLAUDEMD(t *testing.T) {
	dir := t.TempDir()
	prompt := triage.BuildPrompt("", []*db.Comment{makeComment("c1", "a", "b", "f.go", 1)}, dir)

	if prompt == "" {
		t.Error("prompt is empty when CLAUDE.md missing")
	}
	if strings.Contains(prompt, "## Project Guidelines") {
		if !strings.Contains(prompt, "No CLAUDE.md found") {
			t.Error("expected fallback text when CLAUDE.md missing")
		}
	}
}

func TestPromptTruncatesLargeDiff(t *testing.T) {
	hugeDiff := strings.Repeat("+++ b/main.go\n"+strings.Repeat("x", 100)+"\n", 350)

	prompt := triage.BuildPrompt(hugeDiff, []*db.Comment{makeComment("c1", "a", "b", "f.go", 1)}, "")

	if !strings.Contains(prompt, "truncated") {
		t.Error("expected truncation note in prompt for large diff")
	}
	if len(prompt) > 60000 {
		t.Errorf("prompt too large after truncation: %d chars", len(prompt))
	}
}

func TestPromptStructuredOutputRequest(t *testing.T) {
	prompt := triage.BuildPrompt("", []*db.Comment{makeComment("c1", "a", "b", "f.go", 1)}, "")

	if !strings.Contains(strings.ToLower(prompt), "json") {
		t.Error("prompt does not mention JSON output format")
	}
}
