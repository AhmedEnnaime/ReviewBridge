package daemon

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/notify"
	"github.com/ahmedennaime/reviewbridge/internal/platforms"
	"github.com/ahmedennaime/reviewbridge/internal/queue"
	"github.com/ahmedennaime/reviewbridge/internal/session"
	"github.com/ahmedennaime/reviewbridge/internal/triage"
)

type pollerIface interface {
	Start()
	Stop()
	CatchUp()
	DiscoverPRs(*db.Session, string, string) error
}

type triagerIface interface {
	Run([]*db.Comment, string, string) ([]triage.TriageResult, error)
}

type queueWriterIface interface {
	SyncBranch(branch string) error
}

type Deps struct {
	DB           *db.DB
	Poller       pollerIface
	Triage       triagerIface
	Queue        *queue.Queue
	QueueWriter  queueWriterIface
	Notifier     *notify.Notifier
	Registry     *session.Registry
	Platforms    map[string]platforms.Platform
	SessionsPath string
}

type Daemon struct {
	deps    Deps
	pidPath string
	done    chan struct{}
}

func New(deps Deps, pidPath string) *Daemon {
	return &Daemon{
		deps:    deps,
		pidPath: pidPath,
		done:    make(chan struct{}),
	}
}

func (d *Daemon) Start() error {
	if err := writePID(d.pidPath); err != nil {
		return fmt.Errorf("write PID: %w", err)
	}

	if d.deps.Registry != nil && d.deps.SessionsPath != "" {
		d.deps.Registry.SetOnSaved(d.discoverPRsForSession)
		if err := d.deps.Registry.Start(d.deps.SessionsPath); err != nil {
			return fmt.Errorf("start session registry: %w", err)
		}
	}

	d.flushPendingToQueueFiles()

	d.deps.Poller.Start()
	return nil
}

func (d *Daemon) Stop() {
	select {
	case <-d.done:
	default:
		close(d.done)
	}
	d.deps.Poller.Stop()
	if d.deps.Registry != nil {
		d.deps.Registry.Stop() //nolint:errcheck
	}
	removePID(d.pidPath)
	if d.deps.DB != nil {
		d.deps.DB.Close()
	}
}

func (d *Daemon) Run() error {
	log.Printf("[daemon] starting")
	if err := d.Start(); err != nil {
		log.Printf("[daemon] start failed: %v", err)
		return err
	}
	log.Printf("[daemon] started, watching for sessions and comments")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-d.done:
			return nil
		case <-sigCh:
			d.Stop()
			return nil
		case <-tick.C:
			d.processNewComments()
		}
	}
}

func (d *Daemon) ProcessOnce() {
	d.processNewComments()
}

func (d *Daemon) flushPendingToQueueFiles() {
	recoverStates := []string{db.CommentStateInProgress, db.CommentStateStaleSession}

	prs, _ := d.deps.DB.ListOpenPullRequests()
	for _, pr := range prs {
		allComments, _ := d.deps.DB.ListCommentsByPR(pr.PRID)

		var recovered int
		for _, c := range allComments {
			if slices.Contains(recoverStates, c.State) {
				d.deps.DB.UpdateCommentState(c.CommentID, db.CommentStateQueued) //nolint:errcheck
				recovered++
			}
		}

		allComments, _ = d.deps.DB.ListCommentsByPR(pr.PRID)
		queued := filterByState(allComments, db.CommentStateQueued)
		if len(queued) == 0 {
			continue
		}

		d.syncQueueFile(pr.BranchName)
		log.Printf("[daemon] %d queued comment(s) for %s awaiting /check-reviews in Claude Code", len(queued), pr.BranchName)
	}
}

func (d *Daemon) processNewComments() {
	prs, err := d.deps.DB.ListOpenPullRequests()
	if err != nil {
		log.Printf("[daemon] list PRs failed: %v", err)
		return
	}
	for _, pr := range prs {
		allComments, _ := d.deps.DB.ListCommentsByPR(pr.PRID)
		fetched := filterByState(allComments, db.CommentStateFetched)
		if len(fetched) == 0 {
			continue
		}

		log.Printf("[daemon] triaging %d fetched comment(s) for %s", len(fetched), pr.PRID)
		repoPath := d.getRepoPath(pr)
		if _, err := d.deps.Triage.Run(fetched, "", repoPath); err != nil {
			log.Printf("[daemon] triage failed for %s: %v", pr.PRID, err)
			continue
		}

		updated, _ := d.deps.DB.ListCommentsByPR(pr.PRID)
		triaged := filterByState(updated, db.CommentStateTriaged)
		if len(triaged) == 0 {
			continue
		}

		var actionableIDs []string
		for _, c := range triaged {
			if c.TriageVerdict != db.VerdictSkip {
				actionableIDs = append(actionableIDs, c.CommentID)
			}
		}
		skipped := len(triaged) - len(actionableIDs)

		if len(actionableIDs) == 0 {
			log.Printf("[daemon] triage done: all %d comment(s) skipped for %s", skipped, pr.PRID)
			continue
		}

		d.deps.Queue.Enqueue(actionableIDs) //nolint:errcheck
		d.syncQueueFile(pr.BranchName)

		prNum := prNumberFromID(pr.PRID)
		d.deps.Notifier.NotifyComments(
			notify.PR{Number: prNum, Branch: pr.BranchName},
			toNotifyResults(triaged),
		)
		d.deps.Notifier.Notify(
			"ReviewBridge — review comments ready",
			fmt.Sprintf("Run /check-reviews in your Claude Code session for branch %s (%d comment(s))", pr.BranchName, len(actionableIDs)),
		)
		log.Printf("[daemon] triage done: %d queued, %d skipped for %s — run /check-reviews in Claude Code", len(actionableIDs), skipped, pr.BranchName)
	}
}

func (d *Daemon) syncQueueFile(branch string) {
	if d.deps.QueueWriter == nil {
		return
	}
	if err := d.deps.QueueWriter.SyncBranch(branch); err != nil {
		log.Printf("[daemon] failed to sync queue file for branch=%s: %v", branch, err)
	}
}

func (d *Daemon) getRepoPath(pr *db.PullRequest) string {
	if pr.SessionID == nil {
		return ""
	}
	s, _ := d.deps.DB.GetSession(*pr.SessionID)
	if s == nil {
		return ""
	}
	return s.RepoPath
}

func filterByState(comments []*db.Comment, state string) []*db.Comment {
	var out []*db.Comment
	for _, c := range comments {
		if c.State == state {
			out = append(out, c)
		}
	}
	return out
}


func toNotifyResults(comments []*db.Comment) []notify.CommentResult {
	out := make([]notify.CommentResult, len(comments))
	for i, c := range comments {
		out[i] = notify.CommentResult{CommentID: c.CommentID, Verdict: c.TriageVerdict}
	}
	return out
}


func prNumberFromID(prid string) int {
	parts := strings.Split(prid, ":")
	if len(parts) < 3 {
		return 0
	}
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}

func writePID(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600)
}

func removePID(path string) {
	os.Remove(path)
}
