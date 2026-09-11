// Package watch turns filesystem notifications into debounced refresh events
// for worktrees and projects. It uses FSEvents on macOS and inotify on Linux
// via rjeczalik/notify, so one recursive watch per tree is enough.
package watch

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rjeczalik/notify"
)

// Kind says what an event refers to.
type Kind int

const (
	// KindWorktree means files in one worktree changed; Path is the worktree.
	KindWorktree Kind = iota
	// KindProject means repository metadata changed (refs, index); Path is the
	// common git dir and every worktree of that project should refresh.
	KindProject
	// KindWorktreeList means worktrees were added or removed; Path is the
	// common git dir. The caller should rediscover.
	KindWorktreeList
)

// Event is a debounced change notification.
type Event struct {
	Kind Kind
	Path string
}

// Watcher watches a set of worktrees and common git dirs.
type Watcher struct {
	debounce time.Duration
	exclude  []string
	events   chan Event
	raw      chan notify.EventInfo
	done     chan struct{}

	mu        sync.Mutex
	worktrees []string // sorted longest first
	commons   []string // sorted longest first
	timers    map[string]*time.Timer
	watched   map[string]bool
}

// New creates a watcher; call Watch to start it on a set of paths.
func New(debounce time.Duration, exclude []string) *Watcher {
	w := &Watcher{
		debounce: debounce,
		exclude:  exclude,
		events:   make(chan Event, 256),
		raw:      make(chan notify.EventInfo, 4096),
		done:     make(chan struct{}),
		timers:   map[string]*time.Timer{},
		watched:  map[string]bool{},
	}
	go w.loop()
	return w
}

// Events delivers debounced events.
func (w *Watcher) Events() <-chan Event { return w.events }

// Watch replaces the watched set. commons maps a common git dir to nothing
// in particular; only its keys are used. Paths already watched are kept.
func (w *Watcher) Watch(worktrees []string, commons []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.worktrees = cleanSorted(worktrees)
	w.commons = cleanSorted(commons)
	var firstErr error
	for _, p := range w.worktrees {
		if err := w.add(p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, c := range w.commons {
		// A main worktree contains its own .git; the recursive watch on the
		// worktree already covers it.
		if w.coveredByWorktree(c) {
			continue
		}
		if err := w.add(c); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (w *Watcher) coveredByWorktree(p string) bool {
	for _, wt := range w.worktrees {
		if p == wt || strings.HasPrefix(p, wt+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (w *Watcher) add(p string) error {
	if w.watched[p] {
		return nil
	}
	if err := notify.Watch(filepath.Join(p, "..."), w.raw, notify.All); err != nil {
		return err
	}
	w.watched[p] = true
	return nil
}

// Close stops all watches. Events is not closed.
func (w *Watcher) Close() {
	notify.Stop(w.raw)
	close(w.done)
	w.mu.Lock()
	for _, t := range w.timers {
		t.Stop()
	}
	w.mu.Unlock()
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case ei := <-w.raw:
			if ev, ok := w.classify(ei.Path()); ok {
				w.schedule(ev)
			}
		}
	}
}

// classify maps a changed path to an event, or false when it should be ignored.
func (w *Watcher) classify(p string) (Event, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if strings.HasSuffix(p, ".lock") {
		return Event{}, false
	}
	for _, c := range w.commons {
		if !under(p, c) {
			continue
		}
		rel, _ := filepath.Rel(c, p)
		parts := strings.Split(rel, string(filepath.Separator))
		switch parts[0] {
		case "objects", "logs", "lfs", "hooks", "info", "modules":
			return Event{}, false
		case "worktrees":
			if len(parts) <= 2 {
				return Event{Kind: KindWorktreeList, Path: c}, true
			}
			return Event{Kind: KindProject, Path: c}, true
		}
		return Event{Kind: KindProject, Path: c}, true
	}
	for _, wt := range w.worktrees {
		if !under(p, wt) {
			continue
		}
		rel, _ := filepath.Rel(wt, p)
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			if part == ".git" || w.excluded(part) {
				return Event{}, false
			}
		}
		return Event{Kind: KindWorktree, Path: wt}, true
	}
	return Event{}, false
}

func (w *Watcher) excluded(name string) bool {
	for _, pat := range w.exclude {
		pat = strings.TrimPrefix(pat, "**/")
		if ok, _ := filepath.Match(pat, name); ok || pat == name {
			return true
		}
	}
	return false
}

// schedule (re)arms the debounce timer for an event.
func (w *Watcher) schedule(ev Event) {
	key := string(rune('0'+ev.Kind)) + ev.Path
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.timers[key]; ok {
		t.Reset(w.debounce)
		return
	}
	w.timers[key] = time.AfterFunc(w.debounce, func() {
		w.mu.Lock()
		delete(w.timers, key)
		w.mu.Unlock()
		select {
		case w.events <- ev:
		case <-w.done:
		}
	})
}

func under(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

func cleanSorted(ps []string) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if p != "" {
			if real, err := filepath.EvalSymlinks(p); err == nil {
				p = real
			}
			out = append(out, filepath.Clean(p))
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}
