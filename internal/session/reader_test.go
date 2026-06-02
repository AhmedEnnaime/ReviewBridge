package session_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/session"
)

func writeJSONL(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "abc123.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
	return path
}

func TestReaderExtractsRepoPath(t *testing.T) {
	path := writeJSONL(t,
		`{"type":"assistant","timestamp":"2024-01-15T10:30:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repos/myapp/main.go"}}]}}`,
	)
	meta, err := session.ReadMeta(path)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.RepoPath != "/repos/myapp" {
		t.Errorf("RepoPath = %q, want %q", meta.RepoPath, "/repos/myapp")
	}
}

func TestReaderExtractsCWDAsRepoPath(t *testing.T) {
	path := writeJSONL(t,
		`{"type":"user","cwd":"/repos/myapp","timestamp":"2024-01-15T10:30:00Z","message":{"role":"user","content":"hello"}}`,
	)
	meta, err := session.ReadMeta(path)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.RepoPath != "/repos/myapp" {
		t.Errorf("RepoPath = %q, want %q", meta.RepoPath, "/repos/myapp")
	}
}

func TestReaderExtractsSessionID(t *testing.T) {
	path := writeJSONL(t, `{"type":"user","timestamp":"2024-01-15T10:30:00Z"}`)
	meta, err := session.ReadMeta(path)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want %q", meta.SessionID, "abc123")
	}
}

func TestReaderExtractsTimestamp(t *testing.T) {
	path := writeJSONL(t, `{"type":"user","timestamp":"2024-01-15T10:30:00Z"}`)
	meta, err := session.ReadMeta(path)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	want, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	if !meta.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v", meta.CreatedAt, want)
	}
}

func TestReaderEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abc123.jsonl")
	os.WriteFile(path, []byte(""), 0600)

	_, err := session.ReadMeta(path)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

func TestReaderNoToolCalls(t *testing.T) {
	path := writeJSONL(t,
		`{"type":"user","timestamp":"2024-01-15T10:30:00Z","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","timestamp":"2024-01-15T10:30:01Z","message":{"role":"assistant","content":"world"}}`,
	)
	meta, err := session.ReadMeta(path)
	if err != nil {
		t.Fatalf("ReadMeta: %v", err)
	}
	if meta.RepoPath != "" {
		t.Errorf("RepoPath = %q, want empty", meta.RepoPath)
	}
}
