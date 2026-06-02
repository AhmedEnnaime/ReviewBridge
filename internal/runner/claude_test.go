package runner_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/runner"
)

func TestClaudeInstalledOnPath(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "claude")
	os.WriteFile(fake, []byte("#!/bin/sh\necho ok"), 0755)

	r := runner.New().WithLookupFn(func(name string) (string, error) {
		if name == "claude" {
			return fake, nil
		}
		return "", errors.New("not found")
	})

	if !r.IsClaudeInstalled() {
		t.Error("expected IsClaudeInstalled() = true")
	}
}

func TestClaudeNotOnPath(t *testing.T) {
	r := runner.New().WithLookupFn(func(_ string) (string, error) {
		return "", errors.New("not found")
	})

	if r.IsClaudeInstalled() {
		t.Error("expected IsClaudeInstalled() = false")
	}
}

func TestSessionIsActive(t *testing.T) {
	r := runner.New().WithProcessLister(func() ([]string, error) {
		return []string{
			"user   123  0.0  claude --resume abc123 -p fix this",
			"user   456  0.0  other-process",
		}, nil
	})

	if !r.IsSessionActive("abc123") {
		t.Error("expected session abc123 to be active")
	}
}

func TestSessionIsNotActive(t *testing.T) {
	r := runner.New().WithProcessLister(func() ([]string, error) {
		return []string{
			"user   123  0.0  vim main.go",
			"user   456  0.0  go test ./...",
		}, nil
	})

	if r.IsSessionActive("abc123") {
		t.Error("expected session abc123 to not be active")
	}
}

func TestExtractCommitFromOutput(t *testing.T) {
	output := "Making changes...\n[main a3f91bc] Fix null check\n 1 file changed"
	r := runner.New()
	result := r.ExtractCommit(output)
	if result != "a3f91bc" {
		t.Errorf("CommitHash = %q, want a3f91bc", result)
	}
}

func TestExtractCommitNoCommitMade(t *testing.T) {
	output := "Analyzing the code...\nNo changes needed."
	r := runner.New()
	result := r.ExtractCommit(output)
	if result != "" {
		t.Errorf("CommitHash = %q, want empty", result)
	}
}

func TestExtractCommitMultipleMatches(t *testing.T) {
	output := "[main a3f91bc] First commit\nDoing more work...\n[main d4e92cd] Second commit"
	r := runner.New()
	result := r.ExtractCommit(output)
	if result != "d4e92cd" {
		t.Errorf("CommitHash = %q, want last match d4e92cd", result)
	}
}

func TestRunnerPromptIncludesAllComments(t *testing.T) {
	comments := []*db.Comment{
		{CommentID: "c1", Author: "alice", Body: "missing null check", FilePath: "main.go", LineNumber: 10},
		{CommentID: "c2", Author: "bob", Body: "array out of bounds", FilePath: "util.go", LineNumber: 20},
		{CommentID: "c3", Author: "carol", Body: "unused variable", FilePath: "service.go", LineNumber: 30},
	}

	prompt := runner.BuildPrompt(comments)

	for _, c := range comments {
		if !strings.Contains(prompt, c.Body) {
			t.Errorf("prompt missing body: %q", c.Body)
		}
	}
}

func TestRunnerPromptIncludesFileAndLine(t *testing.T) {
	comments := []*db.Comment{
		{CommentID: "c1", Author: "alice", Body: "fix this", FilePath: "internal/auth/handler.go", LineNumber: 42},
	}

	prompt := runner.BuildPrompt(comments)

	if !strings.Contains(prompt, "internal/auth/handler.go") {
		t.Error("prompt missing file path")
	}
	if !strings.Contains(prompt, "42") {
		t.Error("prompt missing line number")
	}
}

func TestRunnerIntegration_HeadlessExecution(t *testing.T) {
	if os.Getenv("REVIEWBRIDGE_INTEGRATION") == "" {
		t.Skip("set REVIEWBRIDGE_INTEGRATION=true to run")
	}
	r := runner.New().WithTimeout(30 * time.Second)
	if !r.IsClaudeInstalled() {
		t.Skip("claude CLI not installed")
	}
	result, err := r.RunHeadless("say the word hello and nothing else")
	if err != nil {
		t.Fatalf("RunHeadless: %v", err)
	}
	if result.Output == "" {
		t.Error("expected non-empty output")
	}
}
