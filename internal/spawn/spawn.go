// Package spawn opens a pane running the mdpane viewer in whatever terminal
// the caller lives in. Because hooks run as children of the agent CLI, the
// inherited environment identifies the right terminal.
//
// The attach-socket model means this runs at most once per session: after a
// pane exists, file switches go over the socket and the pane never restarts.
package spawn

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Terminal string

const (
	Tmux    Terminal = "tmux"
	Zellij  Terminal = "zellij"
	Kitty   Terminal = "kitty"
	WezTerm Terminal = "wezterm"
	ITerm2  Terminal = "iterm2"
	Warp    Terminal = "warp"
	Ghostty Terminal = "ghostty"
	Unknown Terminal = "unknown"
)

// Detect identifies the surrounding terminal from the environment.
// Multiplexers win over the host terminal: inside tmux, split with tmux.
func Detect(getenv func(string) string) Terminal {
	switch {
	case getenv("TMUX") != "":
		return Tmux
	case getenv("ZELLIJ") != "":
		return Zellij
	case getenv("KITTY_LISTEN_ON") != "" || getenv("KITTY_WINDOW_ID") != "":
		return Kitty
	case getenv("WEZTERM_PANE") != "":
		return WezTerm
	}
	switch getenv("TERM_PROGRAM") {
	case "WarpTerminal":
		return Warp
	case "iTerm.app":
		return ITerm2
	case "ghostty":
		return Ghostty
	}
	return Unknown
}

// NeedsRetrySend reports whether the pane cannot receive the target file as
// an argv (so the caller must retry-dial the socket after spawning).
func NeedsRetrySend(t Terminal) bool { return t == Warp }

// ManualInstruction is shown when no programmatic split exists.
func ManualInstruction() string {
	return "mdpane: no split API for this terminal — run 'mdpane attach' in another pane (one time; it stays connected)"
}

// Pane spawns a viewer pane running `exe args...`. For Warp it launches the
// preconfigured tab (see WarpTabConfigPath); args are delivered later over
// the socket by the caller.
func Pane(t Terminal, exe string, args []string) error {
	viewerCmd := shellJoin(append([]string{exe}, args...))
	switch t {
	case Tmux:
		return run("tmux", "split-window", "-h", "-l", "45%", viewerCmd)
	case Zellij:
		zargs := append([]string{"action", "new-pane", "--direction", "right", "--name", "mdpane", "--"}, append([]string{exe}, args...)...)
		return run("zellij", zargs...)
	case WezTerm:
		wargs := append([]string{"cli", "split-pane", "--right", "--percent", "40", "--"}, append([]string{exe}, args...)...)
		return run("wezterm", wargs...)
	case Kitty:
		kargs := append([]string{"@", "launch", "--type=window", "--location=vsplit", "--var", "mdpane=1", "--"}, append([]string{exe}, args...)...)
		if err := run("kitten", kargs...); err != nil {
			return fmt.Errorf("kitty remote control failed (is allow_remote_control enabled in kitty.conf?): %w", err)
		}
		return nil
	case ITerm2:
		script := fmt.Sprintf(`tell application "iTerm2"
  tell current session of current window
    split vertically with default profile command %q
  end tell
end tell`, viewerCmd)
		return run("osascript", "-e", script)
	case Warp:
		if _, err := os.Stat(WarpTabConfigPath()); err != nil {
			return fmt.Errorf("warp tab config missing — run 'mdpane install warp' first")
		}
		return run("open", "warp://tab_config/mdpane?new_window=false")
	default:
		return fmt.Errorf("%s", ManualInstruction())
	}
}

// WarpTabConfigPath is where `mdpane install warp` writes the layout.
func WarpTabConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".warp", "tab_configs", "mdpane.toml")
}

// WarpTabConfig is the shipped layout: shell on the left, viewer on the right.
const WarpTabConfig = `# Installed by 'mdpane install warp'.
# Launch with: open "warp://tab_config/mdpane"
name = "mdpane"

[[panes]]
id = "root"
split = "horizontal"
children = ["shell", "preview"]

[[panes]]
id = "shell"
type = "terminal"

[[panes]]
id = "preview"
type = "terminal"
commands = ["mdpane attach"]
`

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func shellJoin(parts []string) string {
	quoted := make([]string, len(parts))
	for i, p := range parts {
		if strings.ContainsAny(p, " '\"$&|;<>()`\\*?[]#~") {
			quoted[i] = "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
		} else {
			quoted[i] = p
		}
	}
	return strings.Join(quoted, " ")
}
