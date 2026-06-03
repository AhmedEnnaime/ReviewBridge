package daemon

import (
	"log"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/poller"
)

func (d *Daemon) discoverPRsForSession(s *db.Session) {
	if s == nil || s.BranchName == "" || s.RepoPath == "" {
		return
	}
	platformName, repo, err := poller.ParseRemote(s.RepoPath)
	if err != nil {
		log.Printf("[session] cannot parse remote for %s: %v", s.RepoPath, err)
		return
	}
	log.Printf("[session] discovering PRs for session=%s branch=%s repo=%s/%s",
		s.SessionID[:min(8, len(s.SessionID))], s.BranchName, platformName, repo)
	if err := d.deps.Poller.DiscoverPRs(s, platformName, repo); err != nil {
		log.Printf("[session] PR discovery failed for %s: %v", repo, err)
	}
}
