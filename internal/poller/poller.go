package poller

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/platforms"
)

type Poller struct {
	db        *db.DB
	platforms map[string]platforms.Platform
	interval  time.Duration
	tickerFn  func(time.Duration) (<-chan time.Time, func())
	done      chan struct{}
}

func New(d *db.DB, plats map[string]platforms.Platform, interval time.Duration) *Poller {
	return &Poller{
		db:        d,
		platforms: plats,
		interval:  interval,
		tickerFn:  defaultTicker,
		done:      make(chan struct{}),
	}
}

func (p *Poller) WithTickerFn(fn func(time.Duration) (<-chan time.Time, func())) *Poller {
	p.tickerFn = fn
	return p
}

func (p *Poller) Start() {
	p.CatchUp()
	tickC, stop := p.tickerFn(p.interval)
	go func() {
		defer stop()
		for {
			select {
			case <-p.done:
				return
			case <-tickC:
				p.Poll()
			}
		}
	}()
}

func (p *Poller) Stop() {
	close(p.done)
}

func (p *Poller) CatchUp() {
	p.Poll()
}

func (p *Poller) Poll() {
	prs, err := p.db.ListOpenPullRequests()
	if err != nil {
		return
	}
	for _, pr := range prs {
		p.pollPR(pr)
	}
}

func (p *Poller) DiscoverPRs(session *db.Session, platformName, repo string) error {
	plat, ok := p.platforms[platformName]
	if !ok {
		return fmt.Errorf("unknown platform: %s", platformName)
	}
	openPRs, err := plat.ListOpenPullRequests(repo)
	if err != nil {
		log.Printf("[poller] list PRs failed for %s/%s: %v", platformName, repo, err)
		return err
	}
	now := time.Now()
	for _, pr := range openPRs {
		if pr.SourceBranch != session.BranchName {
			continue
		}
		prID := buildPRID(platformName, repo, pr.Number)
		if existing, _ := p.db.GetPullRequest(prID); existing != nil {
			continue
		}
		sid := session.SessionID
		p.db.SavePullRequest(&db.PullRequest{ //nolint:errcheck
			PRID:          prID,
			Platform:      platformName,
			Repo:          repo,
			BranchName:    pr.SourceBranch,
			SessionID:     &sid,
			LastCheckedAt: now,
			Status:        db.PRStatusOpen,
		})
		log.Printf("[poller] discovered PR %s for branch=%s", prID, pr.SourceBranch)
	}
	return nil
}

func (p *Poller) pollPR(pr *db.PullRequest) {
	plat, ok := p.platforms[pr.Platform]
	if !ok {
		return
	}
	number, err := prNumberFromPRID(pr.PRID)
	if err != nil {
		return
	}
	comments, err := plat.ListCommentsSince(pr.Repo, number, pr.LastCheckedAt)
	if err != nil {
		log.Printf("[poller] fetch comments failed for %s: %v", pr.PRID, err)
		return
	}
	if len(comments) > 0 {
		log.Printf("[poller] fetched %d new comment(s) for %s", len(comments), pr.PRID)
	}
	now := time.Now()
	for _, c := range comments {
		dbID := pr.Platform + ":" + c.ID
		if existing, _ := p.db.GetComment(dbID); existing != nil {
			continue
		}
		p.db.SaveComment(&db.Comment{
			CommentID:     dbID,
			PRID:          pr.PRID,
			Author:        c.Author,
			Body:          c.Body,
			FilePath:      c.FilePath,
			LineNumber:    c.Line,
			CreatedAt:     c.CreatedAt,
			FetchedAt:     now,
			TriageVerdict: db.VerdictPending,
			State:         db.CommentStateFetched,
		})
	}
	p.db.UpdateLastChecked(pr.PRID, now)
}

func buildPRID(platform, repo string, number int) string {
	return fmt.Sprintf("%s:%s:%d", platform, repo, number)
}

func prNumberFromPRID(prid string) (int, error) {
	parts := strings.Split(prid, ":")
	if len(parts) < 3 {
		return 0, fmt.Errorf("invalid PRID format: %s", prid)
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func defaultTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}
