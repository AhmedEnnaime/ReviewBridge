package queue_test

import (
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
	"github.com/ahmedennaime/reviewbridge/internal/queue"
)

func newEnv(t *testing.T) (*db.DB, *queue.Queue) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d, queue.New(d)
}

func seedSession(t *testing.T, d *db.DB, sessionID, branch string) {
	t.Helper()
	d.SaveSession(&db.Session{
		SessionID:    sessionID,
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
		PRID:          prID,
		Platform:      "github",
		Repo:          "owner/repo",
		BranchName:    branch,
		SessionID:     &sid,
		LastCheckedAt: time.Now(),
		Status:        db.PRStatusOpen,
	})
}

func seedComment(t *testing.T, d *db.DB, commentID, prID, state string) {
	t.Helper()
	d.SaveComment(&db.Comment{
		CommentID:     commentID,
		PRID:          prID,
		Author:        "alice",
		Body:          "fix this",
		CreatedAt:     time.Now(),
		FetchedAt:     time.Now(),
		TriageVerdict: db.VerdictPending,
		State:         state,
	})
}

func TestEnqueueMovesFromTriagedToQueued(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedPR(t, d, "pr1", "feature/a", "s1")
	for _, id := range []string{"c1", "c2", "c3"} {
		seedComment(t, d, id, "pr1", db.CommentStateTriaged)
	}

	if err := q.Enqueue([]string{"c1", "c2", "c3"}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	for _, id := range []string{"c1", "c2", "c3"} {
		c, _ := d.GetComment(id)
		if c.State != db.CommentStateQueued {
			t.Errorf("comment %s state = %q, want %q", id, c.State, db.CommentStateQueued)
		}
	}
}

func TestEnqueueRejectsWrongState(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedPR(t, d, "pr1", "feature/a", "s1")
	seedComment(t, d, "c1", "pr1", db.CommentStateDone)

	if err := q.Enqueue([]string{"c1"}); err == nil {
		t.Error("expected error enqueuing done comment, got nil")
	}
}

func TestParkMovesFromQueuedToParked(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedPR(t, d, "pr1", "feature/a", "s1")
	for _, id := range []string{"c1", "c2"} {
		seedComment(t, d, id, "pr1", db.CommentStateQueued)
	}

	if err := q.Park([]string{"c1", "c2"}); err != nil {
		t.Fatalf("Park: %v", err)
	}

	for _, id := range []string{"c1", "c2"} {
		c, _ := d.GetComment(id)
		if c.State != db.CommentStateParked {
			t.Errorf("comment %s state = %q, want %q", id, c.State, db.CommentStateParked)
		}
	}
}

func TestUnparkRestoresQueuedState(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedPR(t, d, "pr1", "feature/a", "s1")
	for _, id := range []string{"c1", "c2"} {
		seedComment(t, d, id, "pr1", db.CommentStateParked)
	}

	if err := q.Unpark("feature/a"); err != nil {
		t.Fatalf("Unpark: %v", err)
	}

	for _, id := range []string{"c1", "c2"} {
		c, _ := d.GetComment(id)
		if c.State != db.CommentStateQueued {
			t.Errorf("comment %s state = %q, want %q", id, c.State, db.CommentStateQueued)
		}
	}
}

func TestUnparkOnlyAffectsCorrectBranch(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedSession(t, d, "s2", "feature/b")
	seedPR(t, d, "pr1", "feature/a", "s1")
	seedPR(t, d, "pr2", "feature/b", "s2")
	seedComment(t, d, "c1", "pr1", db.CommentStateParked)
	seedComment(t, d, "c2", "pr2", db.CommentStateParked)

	if err := q.Unpark("feature/b"); err != nil {
		t.Fatalf("Unpark: %v", err)
	}

	c1, _ := d.GetComment("c1")
	if c1.State != db.CommentStateParked {
		t.Errorf("c1 state = %q, want parked (branch A unaffected)", c1.State)
	}
	c2, _ := d.GetComment("c2")
	if c2.State != db.CommentStateQueued {
		t.Errorf("c2 state = %q, want queued (branch B unparked)", c2.State)
	}
}

func TestMarkInProgress(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedPR(t, d, "pr1", "feature/a", "s1")
	seedComment(t, d, "c1", "pr1", db.CommentStateQueued)

	if err := q.MarkInProgress([]string{"c1"}); err != nil {
		t.Fatalf("MarkInProgress: %v", err)
	}

	c, _ := d.GetComment("c1")
	if c.State != db.CommentStateInProgress {
		t.Errorf("state = %q, want %q", c.State, db.CommentStateInProgress)
	}
}

func TestMarkDone(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedPR(t, d, "pr1", "feature/a", "s1")
	seedComment(t, d, "c1", "pr1", db.CommentStateInProgress)

	if err := q.MarkDone([]string{"c1"}, "a3f91bc"); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}

	c, _ := d.GetComment("c1")
	if c.State != db.CommentStateDone {
		t.Errorf("state = %q, want %q", c.State, db.CommentStateDone)
	}
	if c.CommitHash != "a3f91bc" {
		t.Errorf("CommitHash = %q, want a3f91bc", c.CommitHash)
	}
}

func TestListQueued(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedSession(t, d, "s2", "feature/b")
	seedPR(t, d, "pr1", "feature/a", "s1")
	seedPR(t, d, "pr2", "feature/b", "s2")

	for _, id := range []string{"c1", "c2", "c3"} {
		seedComment(t, d, id, "pr1", db.CommentStateQueued)
	}
	for _, id := range []string{"c4", "c5"} {
		seedComment(t, d, id, "pr2", db.CommentStateQueued)
	}

	comments, err := q.ListQueued("s1")
	if err != nil {
		t.Fatalf("ListQueued: %v", err)
	}
	if len(comments) != 3 {
		t.Errorf("got %d comments for s1, want 3", len(comments))
	}
}

func TestListParked(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedSession(t, d, "s2", "feature/b")
	seedPR(t, d, "pr1", "feature/a", "s1")
	seedPR(t, d, "pr2", "feature/b", "s2")
	seedComment(t, d, "c1", "pr1", db.CommentStateParked)
	seedComment(t, d, "c2", "pr1", db.CommentStateParked)
	seedComment(t, d, "c3", "pr2", db.CommentStateParked)

	comments, err := q.ListParked("s1")
	if err != nil {
		t.Fatalf("ListParked: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("got %d parked for s1, want 2", len(comments))
	}
}

func TestDoubleEnqueue(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedPR(t, d, "pr1", "feature/a", "s1")
	seedComment(t, d, "c1", "pr1", db.CommentStateTriaged)

	if err := q.Enqueue([]string{"c1"}); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	if err := q.Enqueue([]string{"c1"}); err != nil {
		t.Fatalf("second Enqueue (double): %v", err)
	}

	comments, _ := d.ListCommentsByPR("pr1")
	if len(comments) != 1 {
		t.Errorf("got %d comments after double enqueue, want 1", len(comments))
	}
}

func TestMarkDoneWithEmptyCommitHash(t *testing.T) {
	d, q := newEnv(t)
	seedSession(t, d, "s1", "feature/a")
	seedPR(t, d, "pr1", "feature/a", "s1")
	seedComment(t, d, "c1", "pr1", db.CommentStateInProgress)

	if err := q.MarkDone([]string{"c1"}, ""); err != nil {
		t.Fatalf("MarkDone with empty hash: %v", err)
	}

	c, _ := d.GetComment("c1")
	if c.State != db.CommentStateDone {
		t.Errorf("state = %q, want %q", c.State, db.CommentStateDone)
	}
}
