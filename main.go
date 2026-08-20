// Command mdpane is a live terminal markdown pane for AI coding agents.
//
//	mdpane [FILE]              view one file (watch + change highlighting)
//	mdpane follow [DIR...]     follow the most recently written markdown file
//	mdpane attach [--open F]   pane mode: viewer + socket server
//	mdpane open FILE           point the running pane at FILE (spawn if needed)
//	mdpane hook claude-code    stdin hook shim for Claude Code
//	mdpane install TARGET      claude-code | opencode | warp
//	mdpane version
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rishabhky/mdpane/internal/ipc"
	"github.com/rishabhky/mdpane/internal/render"
	"github.com/rishabhky/mdpane/internal/spawn"
	"github.com/rishabhky/mdpane/internal/ui"
	"github.com/rishabhky/mdpane/internal/watch"
)

var version = "dev" // set by goreleaser

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mdpane:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return cmdFollow(nil)
	}
	switch args[0] {
	case "follow":
		return cmdFollow(args[1:])
	case "attach":
		return cmdAttach(args[1:])
	case "open":
		return cmdOpen(args[1:])
	case "hook":
		return cmdHook(args[1:])
	case "install":
		return cmdInstall(args[1:])
	case "version", "--version", "-v":
		fmt.Println("mdpane", version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	default:
		if args[0][0] == '-' {
			return fmt.Errorf("unknown flag %q (see 'mdpane help')", args[0])
		}
		return cmdView(args)
	}
}

const usage = `mdpane — live terminal markdown pane for AI coding agents

usage:
  mdpane [FILE]              view one file (watch + change highlighting)
  mdpane follow [DIR...]     follow the newest .md under DIRs (default: . and ~/.claude/plans)
  mdpane attach [--open F]   pane mode: viewer + control socket
  mdpane open FILE           retarget the running pane (spawns one if needed)
  mdpane install TARGET      set up an integration: claude-code | opencode | warp
  mdpane hook claude-code    hook entrypoint (reads Claude Code JSON on stdin)

keys: j/k scroll · g/G top/bottom · f follow · p pin · tab recent · r reload · ? help · q quit
`

func defaultDirs() []string {
	dirs := []string{"."}
	if home, err := os.UserHomeDir(); err == nil {
		plans := filepath.Join(home, ".claude", "plans")
		if info, err := os.Stat(plans); err == nil && info.IsDir() {
			dirs = append(dirs, plans)
		}
	}
	return dirs
}

func startViewer(cfg ui.Config, dirs []string, recursive bool) error {
	r, err := render.New(cfg.Style, 100)
	if err != nil {
		return err
	}
	w, err := watch.New(dirs, watch.Options{Recursive: recursive})
	if err != nil {
		return err
	}
	defer w.Close()
	m := ui.NewModel(cfg, r, w)
	_, err = tea.NewProgram(m).Run()
	return err
}

// cmdView: mdpane FILE — single-file mode.
func cmdView(args []string) error {
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	style := fs.String("style", "dark", "glamour style (dark, light, notty, or a JSON path)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mdpane [--style S] FILE")
	}
	file, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); err != nil {
		return err
	}
	return startViewer(
		ui.Config{InitialFile: file, Style: *style},
		[]string{filepath.Dir(file)},
		false,
	)
}

// cmdFollow: mdpane follow [DIR...] — newest-markdown mode, no socket.
func cmdFollow(args []string) error {
	fs := flag.NewFlagSet("follow", flag.ContinueOnError)
	style := fs.String("style", "dark", "glamour style")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dirs := fs.Args()
	if len(dirs) == 0 {
		dirs = defaultDirs()
	}
	for i, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			return err
		}
		dirs[i] = abs
	}
	return startViewer(
		ui.Config{FollowNewest: true, Style: *style, InitialFile: newestMarkdown(dirs)},
		dirs,
		true,
	)
}

// cmdAttach: pane mode — viewer + socket server, retargetable via `open`.
func cmdAttach(args []string) error {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	style := fs.String("style", "dark", "glamour style")
	openFile := fs.String("open", "", "file to show initially")
	if err := fs.Parse(args); err != nil {
		return err
	}

	srv, err := ipc.Listen()
	if err != nil {
		if err == ipc.ErrBusy {
			return fmt.Errorf("another mdpane pane is already attached (retarget it with 'mdpane open FILE')")
		}
		return err
	}
	defer srv.Close()

	opens := make(chan string, 8)
	go srv.Serve(func(path string) { opens <- path })

	dirs := defaultDirs()
	initial := *openFile
	if initial == "" {
		initial = newestMarkdown(dirs)
	} else if abs, err := filepath.Abs(initial); err == nil {
		initial = abs
		dirs = append(dirs, filepath.Dir(abs))
	}

	for i, d := range dirs {
		if abs, err := filepath.Abs(d); err == nil {
			dirs[i] = abs
		}
	}
	return startViewer(
		ui.Config{
			InitialFile:  initial,
			FollowNewest: true,
			Style:        *style,
			SocketNote:   "attached",
			Opens:        opens,
		},
		dirs,
		true,
	)
}

// cmdOpen: idempotent client — retarget the pane, spawning one if needed.
func cmdOpen(args []string) error {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	noSpawn := fs.Bool("no-spawn", false, "only retarget an existing pane; never spawn one")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mdpane open [--no-spawn] FILE")
	}
	file, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}

	// Fast path: a pane is listening.
	if err := ipc.Send(ipc.Msg{Cmd: "open", Path: file}); err == nil {
		return nil
	}
	if *noSpawn {
		return nil // silently done: hooks use this to avoid pane storms
	}

	term := spawn.Detect(os.Getenv)
	exe, err := os.Executable()
	if err != nil {
		exe = "mdpane"
	}
	if err := spawn.Pane(term, exe, []string{"attach", "--open", file}); err != nil {
		return err
	}
	if spawn.NeedsRetrySend(term) {
		// The Warp tab config launches a plain `mdpane attach`; deliver the
		// target once its socket comes up.
		return ipc.SendWithRetry(ipc.Msg{Cmd: "open", Path: file}, 10*time.Second)
	}
	return nil
}

func newestMarkdown(dirs []string) string {
	var newest string
	var newestMod int64
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if watch.DefaultIgnores[d.Name()] && path != dir {
					return filepath.SkipDir
				}
				return nil
			}
			if !watch.IsMarkdown(path) {
				return nil
			}
			if info, err := d.Info(); err == nil {
				if mt := info.ModTime().UnixNano(); mt > newestMod {
					newestMod, newest = mt, path
				}
			}
			return nil
		})
	}
	return newest
}
