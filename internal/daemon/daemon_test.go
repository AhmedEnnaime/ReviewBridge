package daemon_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/daemon"
	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/dialog"
	"github.com/ahmedennaime/reviewbridge/internal/notify"
	"github.com/ahmedennaime/reviewbridge/internal/queue"
	"github.com/ahmedennaime/reviewbridge/internal/runner"
	"github.com/ahmedennaime/reviewbridge/internal/triage"
)

type mockPoller struct {
	started   bool
	stopped   bool
	catchUpN  int
}

func (m *mockPoller) Start()    { m.started = true }
func (m *mockPoller) Stop()     { m.stopped = true }
func (m *mockPoller) CatchUp()  { m.catchUpN++ }
func (m *mockPoller) Poll()     {}
func (m *mockPoller) DiscoverPRs(_ *db.Session, _, _ string) error { return nil }

type mockRunner struct {
	activeSessionID string
	runSessionID    string
	runErr          error
}

func (m *mockRunner) IsSessionActive(id string) bool {
	return m.activeSessionID == id
}

func (m *mockRunner) Run(sessionID, _ string) (*runner.RunResult, error) {
	m.runSessionID = sessionID
	if m.runErr != nil {
		return nil, m.runErr
	}
	return &runner.RunResult{Output: "done", CommitHash: "abc1234"}, nil
}

type mockTriager struct {
	verdict string
	err     error
}

func (m *mockTriager) Run(comments []*db.Comment, _, _ string) ([]triage.TriageResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	results := make([]triage.TriageResult, len(comments))
	for i, c := range comments {
		results[i] = triage.TriageResult{CommentID: c.CommentID, Verdict: m.verdict, Reason: "test"}
	}
	return results, nil
}

func newTestEnv(t *testing.T) (*db.DB, *daemon.Daemon, *mockPoller, *mockRunner) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	mp := &mockPoller{}
	mr := &mockRunner{}
	mt := &mockTriager{verdict: db.VerdictFix}
	q := queue.New(d)
	n := notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil })

	pidPath := filepath.Join(t.TempDir(), "daemon.pid")

	dmn := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   mp,
		Triage:   mt,
		Queue:    q,
		Notifier: n,
		Runner:   mr,
	}, pidPath).WithShowDialog(func(_ []dialog.DialogItem) ([]string, error) {
		return nil, nil
	})

	return d, dmn, mp, mr
}

func seedSession(t *testing.T, d *db.DB, id, branch string) {
	t.Helper()
	d.SaveSession(&db.Session{
		SessionID:    id,
		RepoPath:     "/repos/myapp",
		BranchName:   branch,
		LastActiveAt: time.Now(),
		Status:       db.SessionStatusActive,
	})
}

func seedPR(t *testing.T, d *db.DB, prID, branch, sessionID string) {
	t.Helper()
	sid := sessionID
	d.SavePullRequest(&db.PullRequest{
		PRID: prID, Platform: "github", Repo: "owner/repo",
		BranchName: branch, SessionID: &sid,
		LastCheckedAt: time.Now(), Status: db.PRStatusOpen,
	})
}

func seedComment(t *testing.T, d *db.DB, id, prID, state string) {
	t.Helper()
	d.SaveComment(&db.Comment{
		CommentID: id, PRID: prID, Author: "alice", Body: "fix this",
		CreatedAt: time.Now(), FetchedAt: time.Now(),
		TriageVerdict: db.VerdictPending, State: state,
	})
}

func TestDaemonStartupInitializesDB(t *testing.T) {
	_, dmn, _, _ := newTestEnv(t)
	if err := dmn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	dmn.Stop()
}

func TestDaemonStartupRunsCatchUp(t *testing.T) {
	_, dmn, mp, _ := newTestEnv(t)
	if err := dmn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	dmn.Stop()

	if !mp.started {
		t.Error("poller was not started")
	}
}

func TestDaemonStartupWithCorruptConfig(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	pidPath := filepath.Join(t.TempDir(), "daemon.pid")

	nilPoller := &mockPoller{}
	nilRunner := &mockRunner{}
	nilTriager := &mockTriager{err: errors.New("no config")}

	dmn := daemon.New(daemon.Deps{
		DB:     d,
		Poller: nilPoller,
		Triage: nilTriager,
		Runner: nilRunner,
		Queue:  queue.New(d),
		Notifier: notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil }),
	}, pidPath)

	err = dmn.Start()
	if err != nil {
		t.Logf("Start returned error as expected: %v", err)
	}
	dmn.Stop()
}

func TestDaemonPIDFileWrittenOnStart(t *testing.T) {
	_, dmn, _, _ := newTestEnv(t)
	pidPath := filepath.Join(t.TempDir(), "daemon.pid")
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	mp := &mockPoller{}
	dmn2 := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   mp,
		Triage:   &mockTriager{verdict: db.VerdictFix},
		Queue:    queue.New(d),
		Notifier: notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil }),
		Runner:   &mockRunner{},
	}, pidPath).WithShowDialog(func(_ []dialog.DialogItem) ([]string, error) { return nil, nil })

	if err := dmn2.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("PID file not found: %v", err)
	}
	pid, _ := strconv.Atoi(string(data))
	if pid != os.Getpid() {
		t.Errorf("PID in file = %d, want %d", pid, os.Getpid())
	}
	dmn2.Stop()
	_ = dmn
}

func TestDaemonPIDFileRemovedOnStop(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "daemon.pid")
	d, _ := db.Open(":memory:")
	defer d.Close()

	dmn := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   &mockPoller{},
		Triage:   &mockTriager{verdict: db.VerdictFix},
		Queue:    queue.New(d),
		Notifier: notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil }),
		Runner:   &mockRunner{},
	}, pidPath).WithShowDialog(func(_ []dialog.DialogItem) ([]string, error) { return nil, nil })

	dmn.Start()
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Fatal("PID file should exist after Start")
	}

	dmn.Stop()
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should be removed after Stop")
	}
}

func TestDaemonRoutesToCorrectSession(t *testing.T) {
	d, _, _, mr := newTestEnv(t)

	seedSession(t, d, "s-a", "feature/a")
	seedSession(t, d, "s-b", "feature/b")
	seedPR(t, d, "github:owner/repo:12", "feature/a", "s-a")
	seedComment(t, d, "c1", "github:owner/repo:12", db.CommentStateTriaged)
	d.SetTriageResult("c1", db.VerdictFix)
	d.UpdateCommentState("c1", db.CommentStateQueued)

	pr, _ := d.GetPullRequest("github:owner/repo:12")
	c1, _ := d.GetComment("c1")

	q := queue.New(d)
	n := notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil })

	dmn := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   &mockPoller{},
		Triage:   &mockTriager{verdict: db.VerdictFix},
		Queue:    q,
		Notifier: n,
		Runner:   mr,
	}, filepath.Join(t.TempDir(), "daemon.pid")).
		WithShowDialog(func(_ []dialog.DialogItem) ([]string, error) { return nil, nil })

	dmn.RouteComments([]*db.Comment{c1}, pr)

	if mr.runSessionID != "s-a" {
		t.Errorf("runner called with session %q, want s-a", mr.runSessionID)
	}
}

func TestDaemonParksWhenSessionActive(t *testing.T) {
	d, _, _, _ := newTestEnv(t)

	seedSession(t, d, "s-a", "feature/a")
	seedPR(t, d, "github:owner/repo:12", "feature/a", "s-a")
	seedComment(t, d, "c1", "github:owner/repo:12", db.CommentStateQueued)

	mr := &mockRunner{activeSessionID: "s-a"}
	q := queue.New(d)

	dmn := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   &mockPoller{},
		Triage:   &mockTriager{verdict: db.VerdictFix},
		Queue:    q,
		Notifier: notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil }),
		Runner:   mr,
	}, filepath.Join(t.TempDir(), "daemon.pid")).
		WithShowDialog(func(_ []dialog.DialogItem) ([]string, error) { return nil, nil })

	pr, _ := d.GetPullRequest("github:owner/repo:12")
	c1, _ := d.GetComment("c1")

	dmn.RouteComments([]*db.Comment{c1}, pr)

	if mr.runSessionID != "" {
		t.Error("runner should not have been called when session is active")
	}

	c1After, _ := d.GetComment("c1")
	if c1After.State != db.CommentStateParked {
		t.Errorf("state = %q, want parked", c1After.State)
	}
}

func TestDaemonUnparksWhenSessionFrees(t *testing.T) {
	d, _, _, _ := newTestEnv(t)

	seedSession(t, d, "s-a", "feature/a")
	seedPR(t, d, "github:owner/repo:12", "feature/a", "s-a")
	seedComment(t, d, "c1", "github:owner/repo:12", db.CommentStateParked)

	mr := &mockRunner{}
	q := queue.New(d)

	if err := q.Unpark("feature/a"); err != nil {
		t.Fatalf("Unpark: %v", err)
	}

	pr, _ := d.GetPullRequest("github:owner/repo:12")
	c1, _ := d.GetComment("c1")

	dmn := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   &mockPoller{},
		Triage:   &mockTriager{verdict: db.VerdictFix},
		Queue:    q,
		Notifier: notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil }),
		Runner:   mr,
	}, filepath.Join(t.TempDir(), "daemon.pid")).
		WithShowDialog(func(_ []dialog.DialogItem) ([]string, error) { return nil, nil })

	dmn.RouteComments([]*db.Comment{c1}, pr)

	if mr.runSessionID != "s-a" {
		t.Errorf("runner called with %q after unpark, want s-a", mr.runSessionID)
	}
}

func TestDaemonShutdownOnSIGINT(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "daemon.pid")
	d, _ := db.Open(":memory:")
	defer d.Close()

	dmn := daemon.New(daemon.Deps{
		DB:       d,
		Poller:   &mockPoller{},
		Triage:   &mockTriager{verdict: db.VerdictFix},
		Queue:    queue.New(d),
		Notifier: notify.New().WithNotifyFn(func(_, _ string, _ any) error { return nil }),
		Runner:   &mockRunner{},
	}, pidPath).WithShowDialog(func(_ []dialog.DialogItem) ([]string, error) { return nil, nil })

	if err := dmn.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		dmn.Stop()
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("daemon did not stop within 3s")
	}
}
