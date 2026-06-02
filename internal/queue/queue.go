package queue

import (
	"fmt"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

type Queue struct {
	db *db.DB
}

func New(d *db.DB) *Queue {
	return &Queue{db: d}
}

func (q *Queue) Enqueue(commentIDs []string) error {
	for _, id := range commentIDs {
		c, err := q.db.GetComment(id)
		if err != nil {
			return err
		}
		if c == nil {
			return fmt.Errorf("comment not found: %s", id)
		}
		switch c.State {
		case db.CommentStateQueued:
			continue
		case db.CommentStateTriaged:
			if err := q.db.UpdateCommentState(id, db.CommentStateQueued); err != nil {
				return err
			}
		default:
			return fmt.Errorf("cannot enqueue comment %q in state %q", id, c.State)
		}
	}
	return nil
}

func (q *Queue) Park(commentIDs []string) error {
	for _, id := range commentIDs {
		if err := q.db.UpdateCommentState(id, db.CommentStateParked); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) Unpark(branchName string) error {
	parked, err := q.db.ListCommentsByStateAndBranch(db.CommentStateParked, branchName)
	if err != nil {
		return err
	}
	for _, c := range parked {
		if err := q.db.UpdateCommentState(c.CommentID, db.CommentStateQueued); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) MarkInProgress(commentIDs []string) error {
	for _, id := range commentIDs {
		if err := q.db.UpdateCommentState(id, db.CommentStateInProgress); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) MarkDone(commentIDs []string, commitHash string) error {
	for _, id := range commentIDs {
		if err := q.db.MarkCommentDone(id, commitHash); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) ListQueued(sessionID string) ([]*db.Comment, error) {
	return q.db.ListCommentsByStateAndSession(db.CommentStateQueued, sessionID)
}

func (q *Queue) ListParked(sessionID string) ([]*db.Comment, error) {
	return q.db.ListCommentsByStateAndSession(db.CommentStateParked, sessionID)
}
