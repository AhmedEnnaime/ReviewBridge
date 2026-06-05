package session

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	fsw       *fsnotify.Watcher
	onSession func(path string)
	done      chan struct{}
}

func NewWatcher(onSession func(path string)) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		fsw:       fsw,
		onSession: onSession,
		done:      make(chan struct{}),
	}
	go w.loop()
	return w, nil
}

func (w *Watcher) Watch(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := w.fsw.Add(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			w.fsw.Add(filepath.Join(dir, entry.Name())) //nolint:errcheck
		}
	}
	return nil
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.fsw.Close()
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(event.Name)
		if err != nil {
			return
		}
		if info.IsDir() {
			w.fsw.Add(event.Name) //nolint:errcheck
			return
		}
	}
	if (event.Has(fsnotify.Create) || event.Has(fsnotify.Write)) && strings.HasSuffix(event.Name, ".jsonl") {
		w.onSession(event.Name)
	}
}
