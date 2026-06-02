package poller

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

func (p *Poller) DiscoverPRsForSession(session *db.Session) error {
	platformName, repo, err := ParseRemote(session.RepoPath)
	if err != nil {
		return err
	}
	return p.DiscoverPRs(session, platformName, repo)
}

func ParseRemote(repoPath string) (platformName, repo string, err error) {
	out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", "", fmt.Errorf("get remote url: %w", err)
	}
	return parseRemoteURL(strings.TrimSpace(string(out)))
}

func parseRemoteURL(rawURL string) (platformName, repo string, err error) {
	if strings.HasPrefix(rawURL, "git@") {
		trimmed := strings.TrimPrefix(rawURL, "git@")
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid SSH URL: %s", rawURL)
		}
		repo = strings.TrimSuffix(parts[1], ".git")
		return platformFromHost(parts[0]), repo, nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("parse remote URL: %w", err)
	}
	repo = strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
	return platformFromHost(u.Host), repo, nil
}

func platformFromHost(host string) string {
	if strings.Contains(host, "gitlab") {
		return "gitlab"
	}
	return "github"
}
