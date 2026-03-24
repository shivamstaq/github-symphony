package config

import (
	"log/slog"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatcherCallback is called when WORKFLOW.md changes with the new parsed workflow.
type WatcherCallback func(wf *WorkflowDefinition)

// Watcher monitors WORKFLOW.md for changes and triggers a callback.
type Watcher struct {
	path     string
	callback WatcherCallback
	watcher  *fsnotify.Watcher
	done     chan struct{}
	mu       sync.Mutex
}

// NewWatcher creates a file watcher with debounced change detection.
func NewWatcher(path string, callback WatcherCallback) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := fw.Add(path); err != nil {
		_ = fw.Close()
		return nil, err
	}

	w := &Watcher{
		path:     path,
		callback: callback,
		watcher:  fw,
		done:     make(chan struct{}),
	}

	go w.loop()
	return w, nil
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	close(w.done)
	return w.watcher.Close()
}

func (w *Watcher) loop() {
	var debounce *time.Timer

	for {
		select {
		case <-w.done:
			if debounce != nil {
				debounce.Stop()
			}
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Debounce: wait 200ms before triggering reload
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(200*time.Millisecond, func() {
					w.reload()
				})
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("watcher error", "error", err)
		}
	}
}

func (w *Watcher) reload() {
	w.mu.Lock()
	defer w.mu.Unlock()

	slog.Info("workflow file changed, reloading", "path", w.path)

	wf, err := LoadWorkflow(w.path)
	if err != nil {
		slog.Error("workflow reload failed, keeping last good config", "error", err)
		return
	}

	w.callback(wf)
}
