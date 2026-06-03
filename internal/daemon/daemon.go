package daemon

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/dialog"
	"github.com/ahmedennaime/reviewbridge/internal/notify"
	"github.com/ahmedennaime/reviewbridge/internal/platforms"
	"github.com/ahmedennaime/reviewbridge/internal/queue"
	"github.com/ahmedennaime/reviewbridge/internal/runner"
	"github.com/ahmedennaime/reviewbridge/internal/session"
	"github.com/ahmedennaime/reviewbridge/internal/triage"
)

type pollerIface interface {
	Start()
	Stop()
	CatchUp()
	DiscoverPRs(*db.Session, string, string) error
}

type runnerIface interface {
	IsSessionActive(string) bool
	Run(string, string) (*runner.RunResult, error)
}

type triagerIface interface {
	Run([]*db.Comment, string, string) ([]triage.TriageResult, error)
}

type Deps struct {
	DB           *db.DB
	Poller       pollerIface
	Triage       triagerIface
	Queue        *queue.Queue
	Notifier     *notify.Notifier
	Runner       runnerIface
	Registry     *session.Registry
	Platforms    map[string]platforms.Platform
	SessionsPath string
}

type Daemon struct {
	deps       Deps
	pidPath    string
	showDialog func([]dialog.DialogItem) ([]string, error)
	done       chan struct{}
}

func New(deps Deps, pidPath string) *Daemon {
	return &Daemon{
		deps:       deps,
		pidPath:    pidPath,
		showDialog: dialog.Show,
		done:       make(chan struct{}),
	}
}

func (d *Daemon) WithShowDialog(fn func([]dialog.DialogItem) ([]string, error)) *Daemon {
	d.showDialog = fn
	return d
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
		d.deps.Registry.Stop()
	}
	removePID(d.pidPath)
	if d.deps.DB != nil {
		d.deps.DB.Close()
	}
}

func (d *Daemon) Run() error {
	if err := d.Start(); err != nil {
		return err
	}

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
			d.unParkReadySessions()
		}
	}
}

func (d *Daemon) RouteComments(comments []*db.Comment, pr *db.PullRequest) {
	if pr == nil || pr.SessionID == nil {
		return
	}
	sessionID := *pr.SessionID
	ids := commentIDs(comments)

	if d.deps.Runner.IsSessionActive(sessionID) {
		d.deps.Queue.Park(ids)
		return
	}

	d.deps.Queue.MarkInProgress(ids)
	prompt := runner.BuildPrompt(comments)
	result, err := d.deps.Runner.Run(sessionID, prompt)
	if err != nil {
		for _, id := range ids {
			d.deps.DB.UpdateCommentState(id, db.CommentStateQueued)
		}
		return
	}
	d.deps.Queue.MarkDone(ids, result.CommitHash)
}

func (d *Daemon) ProcessOnce() {
	d.processNewComments()
	d.unParkReadySessions()
}

func (d *Daemon) processNewComments() {
	prs, err := d.deps.DB.ListOpenPullRequests()
	if err != nil {
		return
	}
	for _, pr := range prs {
		allComments, _ := d.deps.DB.ListCommentsByPR(pr.PRID)
		fetched := filterByState(allComments, db.CommentStateFetched)
		if len(fetched) == 0 {
			continue
		}

		repoPath := d.getRepoPath(pr)
		if _, err := d.deps.Triage.Run(fetched, "", repoPath); err != nil {
			continue
		}

		updated, _ := d.deps.DB.ListCommentsByPR(pr.PRID)
		triaged := filterByState(updated, db.CommentStateTriaged)
		if len(triaged) == 0 {
			continue
		}

		prNum := prNumberFromID(pr.PRID)
		d.deps.Notifier.NotifyComments(
			notify.PR{Number: prNum, Branch: pr.BranchName},
			toNotifyResults(triaged),
		)

		approvedIDs, _ := d.showDialog(toDialogItems(triaged))
		if len(approvedIDs) == 0 {
			continue
		}

		d.deps.Queue.Enqueue(approvedIDs)
		d.RouteComments(filterByIDs(triaged, approvedIDs), pr)
	}
}

func (d *Daemon) unParkReadySessions() {
	sessions, _ := d.deps.DB.ListActiveSessions()
	for _, s := range sessions {
		if d.deps.Runner.IsSessionActive(s.SessionID) {
			continue
		}
		parked, _ := d.deps.Queue.ListParked(s.SessionID)
		if len(parked) == 0 {
			continue
		}
		d.deps.Queue.Unpark(s.BranchName)
		if pr := d.getPRForSession(s.SessionID); pr != nil {
			d.RouteComments(parked, pr)
		}
	}
}

func (d *Daemon) getPRForSession(sessionID string) *db.PullRequest {
	prs, _ := d.deps.DB.ListOpenPullRequests()
	for _, pr := range prs {
		if pr.SessionID != nil && *pr.SessionID == sessionID {
			return pr
		}
	}
	return nil
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

func commentIDs(comments []*db.Comment) []string {
	ids := make([]string, len(comments))
	for i, c := range comments {
		ids[i] = c.CommentID
	}
	return ids
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

func filterByIDs(comments []*db.Comment, ids []string) []*db.Comment {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	var out []*db.Comment
	for _, c := range comments {
		if set[c.CommentID] {
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

func toDialogItems(comments []*db.Comment) []dialog.DialogItem {
	out := make([]dialog.DialogItem, len(comments))
	for i, c := range comments {
		out[i] = dialog.DialogItem{
			CommentID: c.CommentID,
			Author:    c.Author,
			Body:      c.Body,
			FilePath:  c.FilePath,
			Line:      c.LineNumber,
			Verdict:   c.TriageVerdict,
		}
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
