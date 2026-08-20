package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func collectOne(t *testing.T, w *Watcher, timeout time.Duration) Event {
	t.Helper()
	select {
	case ev := <-w.Events():
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for watch event")
		return Event{}
	}
}

func newTestWatcher(t *testing.T, dir string, recursive bool) *Watcher {
	t.Helper()
	w, err := New([]string{dir}, Options{Recursive: recursive, Debounce: 30 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func TestDirectWriteDetected(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(t, dir, false)
	path := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(path, []byte("# hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := collectOne(t, w, 5*time.Second)
	if ev.Path != path {
		t.Fatalf("got %q, want %q", ev.Path, path)
	}
}

func TestAtomicRenameDetected(t *testing.T) {
	// The way agents and editors actually save: write temp, rename over.
	dir := t.TempDir()
	w := newTestWatcher(t, dir, false)
	tmp := filepath.Join(dir, ".plan.md.tmp-x")
	final := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(tmp, []byte("# v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, final); err != nil {
		t.Fatal(err)
	}
	ev := collectOne(t, w, 5*time.Second)
	if ev.Path != final {
		t.Fatalf("got %q, want %q", ev.Path, final)
	}
}

func TestNonMarkdownIgnored(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(t, dir, false)
	if err := os.WriteFile(filepath.Join(dir, "code.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events():
		t.Fatalf("non-markdown file reported: %v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestDebounceCoalesces(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(t, dir, false)
	path := filepath.Join(dir, "doc.md")
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(path, []byte("v"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	collectOne(t, w, 5*time.Second)
	select {
	case <-w.Events():
		t.Fatal("burst of writes was not coalesced into one event")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestRecursiveNewSubdir(t *testing.T) {
	dir := t.TempDir()
	w := newTestWatcher(t, dir, true)
	sub := filepath.Join(dir, "docs")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond) // let the watcher pick up the new dir
	path := filepath.Join(sub, "spec.md")
	if err := os.WriteFile(path, []byte("# spec"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := collectOne(t, w, 5*time.Second)
	if ev.Path != path {
		t.Fatalf("got %q, want %q", ev.Path, path)
	}
}

func TestIgnoredDirsSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := newTestWatcher(t, dir, true)
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "readme.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events():
		t.Fatalf("event from ignored dir: %v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestMaxDirsCapsWatchCount(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 30; i++ {
		if err := os.MkdirAll(filepath.Join(dir, "sub", "d"+string(rune('a'+i%26))+string(rune('a'+i/26))), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w, err := New([]string{dir}, Options{Recursive: true, MaxDirs: 10, Debounce: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if got := w.Watched(); got > 10 {
		t.Fatalf("watched %d dirs, cap is 10", got)
	}
}

func TestMaxDepthLimitsRecursion(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c", "d", "e", "f")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := New([]string{dir}, Options{Recursive: true, MaxDepth: 2, Debounce: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if got := w.Watched(); got > 3 {
		t.Fatalf("watched %d dirs, depth cap should hold it to <=3", got)
	}
}

func TestShallowWatchTopLevelOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := New(nil, Options{Debounce: 30 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if err := w.AddDirShallow(dir); err != nil {
		t.Fatal(err)
	}
	// Top-level write: detected.
	top := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(top, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ev := collectOne(t, w, 5*time.Second)
	if ev.Path != top {
		t.Fatalf("got %q want %q", ev.Path, top)
	}
	// Nested write: NOT detected (shallow).
	if err := os.WriteFile(filepath.Join(dir, "sub", "deep.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events():
		t.Fatalf("shallow watch caught nested file: %v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}
