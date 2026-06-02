package session

import (
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

type Registry struct {
	db        *db.DB
	watcher   *Watcher
	getBranch func(string) (string, error)
}

func NewRegistry(d *db.DB) *Registry {
	return &Registry{
		db:        d,
		getBranch: GetBranch,
	}
}

func (r *Registry) SetBranchFn(fn func(string) (string, error)) {
	r.getBranch = fn
}

func (r *Registry) Start(sessionsPath string) error {
	w, err := NewWatcher(r.handleNewSession)
	if err != nil {
		return err
	}
	r.watcher = w
	return w.Watch(sessionsPath)
}

func (r *Registry) Stop() error {
	if r.watcher != nil {
		return r.watcher.Close()
	}
	return nil
}

func (r *Registry) handleNewSession(path string) {
	meta, err := ReadMeta(path)
	if err != nil {
		return
	}

	existing, _ := r.db.GetSession(meta.SessionID)
	if existing != nil {
		return
	}

	branch := ""
	if meta.RepoPath != "" {
		branch, _ = r.getBranch(meta.RepoPath)
	}

	createdAt := meta.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	r.db.SaveSession(&db.Session{
		SessionID:    meta.SessionID,
		RepoPath:     meta.RepoPath,
		BranchName:   branch,
		LastActiveAt: createdAt,
		Status:       db.SessionStatusActive,
	})
}
