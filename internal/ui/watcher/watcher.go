package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event represents a debounced external file change.
type Event struct {
	Path string // relative to the watched root
	Op   string // "create", "write", "remove", "rename"
}

// Watcher monitors a directory tree for external file changes, debouncing
// rapid events and filtering out agent-originated modifications.
type Watcher struct {
	fsw  *fsnotify.Watcher
	root string

	// agentFiles tracks files recently modified by the agent.
	// Key: absolute path, Value: timestamp of last agent modification.
	agentMu    sync.Mutex
	agentFiles map[string]time.Time

	// eventCh delivers batches of external file changes.
	eventCh chan []Event

	done chan struct{}
}

// New creates a watcher for the given root directory.
// Events are delivered on the channel returned by Events().
func New(root string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		fsw:        fsw,
		root:       root,
		agentFiles: make(map[string]time.Time),
		eventCh:    make(chan []Event, 8),
		done:       make(chan struct{}),
	}
	if err := w.addRecursive(root); err != nil {
		fsw.Close()
		return nil, err
	}
	go w.loop()
	return w, nil
}

// Events returns the channel that delivers batched external file changes.
func (w *Watcher) Events() <-chan []Event {
	return w.eventCh
}

// MarkAgentFile records that the agent just modified a file, so the next
// fsnotify event on it will be suppressed.
func (w *Watcher) MarkAgentFile(absPath string) {
	w.agentMu.Lock()
	w.agentFiles[absPath] = time.Now()
	w.agentMu.Unlock()
}

// Close shuts down the watcher.
func (w *Watcher) Close() {
	w.fsw.Close()
	<-w.done // wait for loop to exit
}

const (
	debounceWindow = 500 * time.Millisecond
	agentGrace     = 2 * time.Second // how long to suppress agent-originated events
)

func (w *Watcher) loop() {
	defer close(w.done)
	defer close(w.eventCh)

	pending := make(map[string]fsnotify.Event)
	var timer *time.Timer
	var timerC <-chan time.Time

	for {
		select {
		case ev, ok := <-w.fsw.Events:
			if !ok {
				// Watcher closed — flush any pending events.
				if len(pending) > 0 {
					w.flush(pending)
				}
				return
			}
			if w.shouldIgnore(ev.Name) {
				continue
			}
			// If a new directory was created, start watching it.
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(ev.Name)
					continue // directory creation itself isn't interesting
				}
			}
			pending[ev.Name] = ev
			if timer == nil {
				timer = time.NewTimer(debounceWindow)
				timerC = timer.C
			} else {
				timer.Reset(debounceWindow)
			}

		case <-timerC:
			timer = nil
			timerC = nil
			if len(pending) > 0 {
				w.flush(pending)
				pending = make(map[string]fsnotify.Event)
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// Log errors but keep running — transient errors are common.
			_ = err
		}
	}
}

func (w *Watcher) flush(pending map[string]fsnotify.Event) {
	now := time.Now()
	var events []Event

	w.agentMu.Lock()
	// Expire old agent file entries.
	for path, ts := range w.agentFiles {
		if now.Sub(ts) > agentGrace {
			delete(w.agentFiles, path)
		}
	}
	for absPath, ev := range pending {
		// Skip if the agent recently modified this file.
		if ts, ok := w.agentFiles[absPath]; ok && now.Sub(ts) <= agentGrace {
			delete(w.agentFiles, absPath) // consumed
			continue
		}
		rel, err := filepath.Rel(w.root, absPath)
		if err != nil {
			rel = absPath
		}
		events = append(events, Event{
			Path: rel,
			Op:   opString(ev.Op),
		})
	}
	w.agentMu.Unlock()

	if len(events) > 0 {
		select {
		case w.eventCh <- events:
		default:
			// Channel full — drop oldest batch, send new one.
			select {
			case <-w.eventCh:
			default:
			}
			select {
			case w.eventCh <- events:
			default:
			}
		}
	}
}

func (w *Watcher) shouldIgnore(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return true
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, p := range parts {
		switch {
		case p == ".git":
			return true
		case p == "node_modules":
			return true
		case p == "__pycache__":
			return true
		case p == ".golem":
			return true
		case strings.HasPrefix(p, ".#"): // Emacs lock files
			return true
		}
	}
	base := filepath.Base(path)
	switch {
	case strings.HasSuffix(base, "~"):
		return true
	case strings.HasSuffix(base, ".swp") || strings.HasSuffix(base, ".swo"):
		return true
	case strings.HasPrefix(base, ".") && strings.HasSuffix(base, ".swp"):
		return true
	case base == "4913": // Vim temp file test
		return true
	case strings.HasPrefix(base, "#") && strings.HasSuffix(base, "#"):
		return true // Emacs auto-save
	}
	return false
}

func (w *Watcher) addRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".git" || name == "node_modules" || name == "__pycache__" || name == ".golem" {
			return filepath.SkipDir
		}
		return w.fsw.Add(path)
	})
}

func opString(op fsnotify.Op) string {
	switch {
	case op.Has(fsnotify.Create):
		return "create"
	case op.Has(fsnotify.Write):
		return "write"
	case op.Has(fsnotify.Remove):
		return "remove"
	case op.Has(fsnotify.Rename):
		return "rename"
	default:
		return "modify"
	}
}
