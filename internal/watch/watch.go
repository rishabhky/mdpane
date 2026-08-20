// Package watch turns fsnotify into what mdpane actually needs: "tell me,
// debounced, when a matching file changes under these directories".
//
// It always watches directories, never files: agents and editors write via
// temp-then-rename, which replaces the inode and silently kills a watch on
// the file path itself (fsnotify#372). Renames/creates of a matching name
// count as changes.
package watch

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event reports that a matching file changed.
type Event struct {
	Path string
}

// DefaultIgnores are directory names never descended into.
var DefaultIgnores = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"target": true, ".venv": true, "__pycache__": true, ".cache": true,
	".next": true, "build": true,
}

// IsMarkdown is the default match function.
func IsMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown":
		return true
	}
	return false
}

type Options struct {
	Recursive bool
	Match     func(path string) bool // default IsMarkdown
	Debounce  time.Duration          // default 150ms
	Ignores   map[string]bool        // default DefaultIgnores
	// MaxDirs caps watched directories (default 2048). On macOS each
	// watched dir costs a kqueue file descriptor, so an unbounded
	// recursive walk of a big tree exhausts the fd limit and can kill
	// the process. Beyond the cap, deeper dirs are simply not watched.
	MaxDirs int
	// MaxDepth caps recursion depth below each root (default 8).
	MaxDepth int
}

type Watcher struct {
	fs     *fsnotify.Watcher
	opts   Options
	events chan Event

	mu      sync.Mutex
	timers  map[string]*time.Timer
	watched int
	closed  bool
}

// Watched reports how many directories are being watched.
func (w *Watcher) Watched() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.watched
}

func New(dirs []string, opts Options) (*Watcher, error) {
	if opts.Match == nil {
		opts.Match = IsMarkdown
	}
	if opts.Debounce <= 0 {
		opts.Debounce = 150 * time.Millisecond
	}
	if opts.Ignores == nil {
		opts.Ignores = DefaultIgnores
	}
	if opts.MaxDirs <= 0 {
		opts.MaxDirs = 2048
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 8
	}
	fs, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		fs:     fs,
		opts:   opts,
		events: make(chan Event, 64),
		timers: make(map[string]*time.Timer),
	}
	for _, d := range dirs {
		if err := w.AddDir(d); err != nil {
			fs.Close()
			return nil, err
		}
	}
	go w.loop()
	return w, nil
}

// Events delivers debounced change notifications.
func (w *Watcher) Events() <-chan Event { return w.events }

// AddDir starts watching a directory (recursively if configured), within
// the MaxDirs / MaxDepth budgets.
func (w *Watcher) AddDir(dir string) error {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		if err == nil {
			return nil
		}
		return err
	}
	if !w.opts.Recursive {
		return w.add(dir)
	}
	rootDepth := strings.Count(dir, string(filepath.Separator))
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, don't fail the watch
		}
		if !d.IsDir() {
			return nil
		}
		if w.opts.Ignores[d.Name()] || (strings.HasPrefix(d.Name(), ".") && path != dir) {
			return filepath.SkipDir
		}
		if strings.Count(path, string(filepath.Separator))-rootDepth >= w.opts.MaxDepth {
			return filepath.SkipDir
		}
		if w.Watched() >= w.opts.MaxDirs {
			return filepath.SkipAll
		}
		_ = w.add(path)
		return nil
	})
}

// AddDirShallow watches a single directory level, regardless of the
// Recursive option — for high-churn trees like ~/Downloads where only
// top-level files matter.
func (w *Watcher) AddDirShallow(dir string) error {
	dir = filepath.Clean(dir)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return err
	}
	return w.add(dir)
}

func (w *Watcher) add(dir string) error {
	if err := w.fs.Add(dir); err != nil {
		return err
	}
	w.mu.Lock()
	w.watched++
	w.mu.Unlock()
	return nil
}

func (w *Watcher) loop() {
	for {
		select {
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			w.handle(ev)
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			// Watch errors are non-fatal for a preview tool; keep going.
		}
	}
}

func (w *Watcher) handle(ev fsnotify.Event) {
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
		return
	}
	// New directory: extend the watch (agents create docs/ dirs mid-run).
	if w.opts.Recursive && ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if !w.opts.Ignores[filepath.Base(ev.Name)] {
				_ = w.AddDir(ev.Name)
			}
			return
		}
	}
	if !w.opts.Match(ev.Name) {
		return
	}
	path := filepath.Clean(ev.Name)

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	if t, ok := w.timers[path]; ok {
		t.Reset(w.opts.Debounce)
		return
	}
	w.timers[path] = time.AfterFunc(w.opts.Debounce, func() {
		w.mu.Lock()
		delete(w.timers, path)
		closed := w.closed
		w.mu.Unlock()
		if closed {
			return
		}
		// Only report files that still exist (rename-away emits too).
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return
		}
		select {
		case w.events <- Event{Path: path}:
		default: // consumer wedged; dropping is fine for a previewer
		}
	})
}

func (w *Watcher) Close() error {
	w.mu.Lock()
	w.closed = true
	for _, t := range w.timers {
		t.Stop()
	}
	w.mu.Unlock()
	return w.fs.Close()
}
