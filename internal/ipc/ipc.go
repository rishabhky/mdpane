// Package ipc implements the viewer-is-the-server socket protocol.
//
// The running viewer listens on a unix socket; `mdpane open FILE` dials it
// and sends one newline-delimited JSON message. A flock guards the
// dial-then-listen race and, being released on process death, can never go
// stale the way PID files do. Prior art: lf -remote, the gopls daemon.
package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// Msg is the wire format (ndjson, one message per connection).
type Msg struct {
	Cmd  string `json:"cmd"`            // "open" | "ping"
	Path string `json:"path,omitempty"` // absolute path for "open"
}

type reply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// ErrBusy means another viewer already owns the socket.
var ErrBusy = errors.New("ipc: another mdpane viewer is already serving")

// maxSockPath stays safely under sun_path (104 bytes on macOS, 108 Linux).
const maxSockPath = 96

// SocketPath prefers XDG_RUNTIME_DIR but falls back to short, stable
// locations when the resulting path would blow the sun_path limit.
func SocketPath() string {
	name := fmt.Sprintf("mdpane-%d.sock", os.Getuid())
	candidates := []string{os.Getenv("XDG_RUNTIME_DIR"), os.TempDir(), "/tmp"}
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if len(p) <= maxSockPath {
			return p
		}
	}
	return filepath.Join("/tmp", name)
}

func lockPath() string { return SocketPath() + ".lock" }

// Server owns the socket for the lifetime of a viewer.
type Server struct {
	ln   net.Listener
	lock *flock.Flock
}

// Listen claims the socket. ErrBusy if a live viewer already holds it.
func Listen() (*Server, error) {
	lk := flock.New(lockPath())
	got, err := lk.TryLock()
	if err != nil {
		return nil, err
	}
	if !got {
		return nil, ErrBusy
	}
	// We hold the lock: any existing socket file is stale.
	_ = os.Remove(SocketPath())
	ln, err := net.Listen("unix", SocketPath())
	if err != nil {
		_ = lk.Unlock()
		return nil, err
	}
	return &Server{ln: ln, lock: lk}, nil
}

// Serve accepts messages until Close; onOpen is called for "open" commands.
func (s *Server) Serve(onOpen func(path string)) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			var m Msg
			if err := json.NewDecoder(bufio.NewReader(c)).Decode(&m); err != nil {
				writeReply(c, reply{OK: false, Error: "bad message"})
				return
			}
			switch m.Cmd {
			case "ping":
				writeReply(c, reply{OK: true})
			case "open":
				if m.Path == "" {
					writeReply(c, reply{OK: false, Error: "open needs a path"})
					return
				}
				onOpen(m.Path)
				writeReply(c, reply{OK: true})
			default:
				writeReply(c, reply{OK: false, Error: "unknown cmd " + m.Cmd})
			}
		}(conn)
	}
}

func (s *Server) Close() {
	_ = s.ln.Close()
	_ = os.Remove(SocketPath())
	_ = s.lock.Unlock()
}

func writeReply(c net.Conn, r reply) {
	buf, _ := json.Marshal(r)
	_, _ = c.Write(append(buf, '\n'))
}

// Send delivers one message to a running viewer.
func Send(m Msg) error {
	conn, err := net.DialTimeout("unix", SocketPath(), time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	buf, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(buf, '\n')); err != nil {
		return err
	}
	var r reply
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&r); err != nil {
		return err
	}
	if !r.OK {
		return errors.New("ipc: viewer refused: " + r.Error)
	}
	return nil
}

// SendWithRetry keeps dialing while a freshly spawned pane starts up.
func SendWithRetry(m Msg, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := Send(m)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}
