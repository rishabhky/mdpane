package ipc

import (
	"os"
	"testing"
	"time"
)

// Point the socket somewhere private AND short: sun_path is ~104 bytes and
// t.TempDir() on macOS easily exceeds it.
func testSocket(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mdpt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
}

func TestOpenRoundTrip(t *testing.T) {
	testSocket(t)
	srv, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	got := make(chan string, 1)
	go srv.Serve(func(path string) { got <- path }, nil)

	if err := Send(Msg{Cmd: "open", Path: "/tmp/x.md"}); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-got:
		if p != "/tmp/x.md" {
			t.Fatalf("got %q", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("open never delivered")
	}
}

func TestSecondListenerRefused(t *testing.T) {
	testSocket(t)
	srv, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if _, err := Listen(); err != ErrBusy {
		t.Fatalf("second Listen: got %v, want ErrBusy", err)
	}
}

func TestSendWithoutViewerFails(t *testing.T) {
	testSocket(t)
	if err := Send(Msg{Cmd: "ping"}); err == nil {
		t.Fatal("Send with no viewer should error")
	}
}

func TestStaleSocketRecovered(t *testing.T) {
	testSocket(t)
	// Simulate a crashed viewer: socket file exists, no listener, no lock.
	if err := os.WriteFile(SocketPath(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := Listen()
	if err != nil {
		t.Fatalf("stale socket not recovered: %v", err)
	}
	srv.Close()
}

func TestSendWithRetryEventuallyConnects(t *testing.T) {
	testSocket(t)
	go func() {
		time.Sleep(300 * time.Millisecond)
		srv, err := Listen()
		if err != nil {
			return
		}
		go srv.Serve(func(string) {}, nil)
		time.Sleep(2 * time.Second)
		srv.Close()
	}()
	if err := SendWithRetry(Msg{Cmd: "ping"}, 5*time.Second); err != nil {
		t.Fatalf("retry never connected: %v", err)
	}
}

func TestTakeoverEvictsIncumbent(t *testing.T) {
	testSocket(t)
	old, err := Listen()
	if err != nil {
		t.Fatal(err)
	}
	evicted := make(chan struct{})
	go old.Serve(func(string) {}, func() {
		// Real flow: the old process's UI quits and the deferred Close
		// releases the lock. Simulate that.
		go func() { old.Close(); close(evicted) }()
	})

	niu, err := TakeoverListen()
	if err != nil {
		t.Fatalf("takeover failed: %v", err)
	}
	defer niu.Close()
	select {
	case <-evicted:
	case <-time.After(3 * time.Second):
		t.Fatal("incumbent was never evicted")
	}
	// New server must actually work.
	got := make(chan string, 1)
	go niu.Serve(func(p string) { got <- p }, nil)
	if err := Send(Msg{Cmd: "open", Path: "/tmp/z.md"}); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-got:
		if p != "/tmp/z.md" {
			t.Fatalf("got %q", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("new server not serving")
	}
}

func TestTakeoverWithNoIncumbentJustListens(t *testing.T) {
	testSocket(t)
	srv, err := TakeoverListen()
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
}
