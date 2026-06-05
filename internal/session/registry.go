package session

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

type Registry struct {
	db        *db.DB
	watcher   *Watcher
	getBranch func(string) (string, error)
	onSaved   func(*db.Session)
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

func (r *Registry) SetOnSaved(fn func(*db.Session)) {
	r.onSaved = fn
}

func (r *Registry) Start(sessionsPath string) error {
	w, err := NewWatcher(r.handleNewSession)
	if err != nil {
		return err
	}
	r.watcher = w
	if err := w.Watch(sessionsPath); err != nil {
		return err
	}
	r.scanExisting(sessionsPath)
	return nil
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
	if existing != nil && existing.RepoPath != "" && existing.BranchName != "" {
		return
	}

	branch := ""
	if meta.RepoPath != "" {
		branch, _ = r.getBranch(meta.RepoPath)
	}

	if meta.RepoPath == "" || branch == "" {
		return
	}

	createdAt := meta.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	s := &db.Session{
		SessionID:    meta.SessionID,
		RepoPath:     meta.RepoPath,
		BranchName:   branch,
		LastActiveAt: createdAt,
		Status:       db.SessionStatusActive,
	}
	r.db.SaveSession(s) //nolint:errcheck
	log.Printf("[session] detected session=%s branch=%s repo=%s",
		meta.SessionID[:min(8, len(meta.SessionID))], branch, meta.RepoPath)

	if r.onSaved != nil {
		r.onSaved(s)
	}
}

func (r *Registry) scanExisting(sessionsPath string) {
	entries, err := os.ReadDir(sessionsPath)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(sessionsPath, entry.Name())
		files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		for _, f := range files {
			r.handleNewSession(f)
		}
	}
}
