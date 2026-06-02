package session

import (
	"errors"
	"os/exec"
	"strings"
)

var (
	ErrNotGitRepo = errors.New("not a git repository")
	ErrNoBranch   = errors.New("could not determine branch: detached HEAD or empty repository")
)

func GetBranch(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "branch", "--show-current").Output()
	if err != nil {
		stderr := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		if strings.Contains(stderr, "not a git repository") {
			return "", ErrNotGitRepo
		}
		return "", err
	}

	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", ErrNoBranch
	}
	return branch, nil
}
