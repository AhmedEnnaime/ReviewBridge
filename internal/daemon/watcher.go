package daemon

import (
	"github.com/ahmedennaime/reviewbridge/internal/poller"
	"github.com/ahmedennaime/reviewbridge/internal/session"
)

func (d *Daemon) onNewSession(path string) {
	meta, err := session.ReadMeta(path)
	if err != nil || meta.RepoPath == "" || meta.SessionID == "" {
		return
	}

	s, _ := d.deps.DB.GetSession(meta.SessionID)
	if s == nil || s.BranchName == "" {
		return
	}

	platformName, repo, err := poller.ParseRemote(s.RepoPath)
	if err != nil {
		return
	}

	d.deps.Poller.DiscoverPRs(s, platformName, repo)
}
