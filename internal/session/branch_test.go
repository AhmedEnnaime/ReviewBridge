package session_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ahmedennaime/reviewbridge/internal/session"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	return dir
}

func TestGetBranchFromValidRepo(t *testing.T) {
	dir := initGitRepo(t)
	f, _ := os.Create(filepath.Join(dir, "readme.txt"))
	f.Close()
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	branch, err := session.GetBranch(dir)
	if err != nil {
		t.Fatalf("GetBranch: %v", err)
	}
	if branch == "" {
		t.Error("expected non-empty branch name")
	}
}

func TestGetBranchDetachedHEAD(t *testing.T) {
	dir := initGitRepo(t)
	f, _ := os.Create(filepath.Join(dir, "readme.txt"))
	f.Close()
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	out, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	hash := string(out[:len(out)-1])
	exec.Command("git", "-C", dir, "checkout", hash).Run()

	_, err := session.GetBranch(dir)
	if !errors.Is(err, session.ErrNoBranch) {
		t.Errorf("error = %v, want ErrNoBranch", err)
	}
}

func TestGetBranchNotARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := session.GetBranch(dir)
	if !errors.Is(err, session.ErrNotGitRepo) {
		t.Errorf("error = %v, want ErrNotGitRepo", err)
	}
}

func TestGetBranchEmptyRepo(t *testing.T) {
	dir := initGitRepo(t)
	branch, err := session.GetBranch(dir)
	if err != nil {
		t.Fatalf("GetBranch on empty repo: %v", err)
	}
	if branch == "" {
		t.Error("expected non-empty default branch name on empty repo")
	}
}
