package daemon

import (
	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/poller"
)

func (d *Daemon) discoverPRsForSession(s *db.Session) {
	if s == nil || s.BranchName == "" || s.RepoPath == "" {
		return
	}
	platformName, repo, err := poller.ParseRemote(s.RepoPath)
	if err != nil {
		return
	}
	d.deps.Poller.DiscoverPRs(s, platformName, repo) //nolint:errcheck
}
