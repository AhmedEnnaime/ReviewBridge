package session_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/session"
)

func TestWatcherDetectsNewSession(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "myproject")
	os.MkdirAll(subdir, 0755)

	called := make(chan string, 1)
	w, err := session.NewWatcher(func(path string) {
		called <- path
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := w.Watch(dir); err != nil {
		t.Fatalf("Watch: %v", err)
	}

	os.WriteFile(filepath.Join(subdir, "abc123.jsonl"), []byte(`{}`), 0600)

	select {
	case path := <-called:
		if !strings.HasSuffix(path, "abc123.jsonl") {
			t.Errorf("unexpected path: %s", path)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout: watcher did not fire within 2s")
	}
}
