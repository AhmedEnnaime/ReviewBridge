package queue

import (
	"fmt"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

type Queue struct {
	db       *db.DB
	onChange func(commentIDs []string)
}

func New(d *db.DB) *Queue {
	return &Queue{db: d}
}

func (q *Queue) WithOnChange(fn func(commentIDs []string)) *Queue {
	q.onChange = fn
	return q
}

func (q *Queue) notifyChange(ids []string) {
	if q.onChange != nil && len(ids) > 0 {
		q.onChange(ids)
	}
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
	q.notifyChange(commentIDs)
	return nil
}

func (q *Queue) Park(commentIDs []string) error {
	for _, id := range commentIDs {
		if err := q.db.UpdateCommentState(id, db.CommentStateParked); err != nil {
			return err
		}
	}
	q.notifyChange(commentIDs)
	return nil
}

func (q *Queue) ParkStale(commentIDs []string) error {
	for _, id := range commentIDs {
		if err := q.db.UpdateCommentState(id, db.CommentStateStaleSession); err != nil {
			return err
		}
	}
	q.notifyChange(commentIDs)
	return nil
}

func (q *Queue) Unpark(branchName string) error {
	parked, err := q.db.ListCommentsByStateAndBranch(db.CommentStateParked, branchName)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(parked))
	for _, c := range parked {
		if err := q.db.UpdateCommentState(c.CommentID, db.CommentStateQueued); err != nil {
			return err
		}
		ids = append(ids, c.CommentID)
	}
	q.notifyChange(ids)
	return nil
}

func (q *Queue) MarkInProgress(commentIDs []string) error {
	for _, id := range commentIDs {
		if err := q.db.UpdateCommentState(id, db.CommentStateInProgress); err != nil {
			return err
		}
	}
	q.notifyChange(commentIDs)
	return nil
}

func (q *Queue) MarkDone(commentIDs []string, commitHash string) error {
	for _, id := range commentIDs {
		if err := q.db.MarkCommentDone(id, commitHash); err != nil {
			return err
		}
	}
	q.notifyChange(commentIDs)
	return nil
}

func (q *Queue) ListQueued(sessionID string) ([]*db.Comment, error) {
	return q.db.ListCommentsByStateAndSession(db.CommentStateQueued, sessionID)
}

func (q *Queue) ListParked(sessionID string) ([]*db.Comment, error) {
	return q.db.ListCommentsByStateAndSession(db.CommentStateParked, sessionID)
}
