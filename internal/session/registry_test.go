package session_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/session"
)

func newTestRegistry(t *testing.T, branchFn func(string) (string, error)) (*session.Registry, *db.DB) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	r := session.NewRegistry(d)
	r.SetBranchFn(branchFn)
	return r, d
}

func validJSONL() string {
	return `{"type":"user","cwd":"/repos/myapp","timestamp":"2024-01-15T10:30:00Z","message":{"role":"user","content":"hello"}}` + "\n"
}

func TestRegistryCreatesSessionOnNewFile(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "myproject")
	os.MkdirAll(subdir, 0755)

	r, d := newTestRegistry(t, func(string) (string, error) {
		return "feature/issue-a", nil
	})
	if err := r.Start(dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	path := filepath.Join(subdir, "abc123.jsonl")
	os.WriteFile(path, []byte(validJSONL()), 0600)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s, _ := d.GetSession("abc123")
		if s != nil {
			if s.BranchName != "feature/issue-a" {
				t.Errorf("BranchName = %q, want feature/issue-a", s.BranchName)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("timeout: session not saved to DB within 2s")
}

func TestRegistryIgnoresNonJSONLFiles(t *testing.T) {
	dir := t.TempDir()
	r, d := newTestRegistry(t, func(string) (string, error) {
		return "main", nil
	})
	if err := r.Start(dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0600)
	time.Sleep(200 * time.Millisecond)

	sessions, err := d.ListActiveSessions()
	if err != nil {
		t.Fatalf("ListActiveSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("got %d sessions, want 0", len(sessions))
	}
}

func TestRegistryIgnoresDuplicates(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "myproject")
	os.MkdirAll(subdir, 0755)

	r, d := newTestRegistry(t, func(string) (string, error) {
		return "main", nil
	})
	if err := r.Start(dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer r.Stop()

	path := filepath.Join(subdir, "abc123.jsonl")
	os.WriteFile(path, []byte(validJSONL()), 0600)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s, _ := d.GetSession("abc123")
		if s != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	os.WriteFile(path, []byte(validJSONL()+"extra line\n"), 0600)
	time.Sleep(300 * time.Millisecond)

	sessions, _ := d.ListActiveSessions()
	if len(sessions) != 1 {
		t.Errorf("got %d sessions after duplicate write, want 1", len(sessions))
	}
}
