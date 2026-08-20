package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rishabhky/mdpane/internal/spawn"
)

func cmdInstall(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: mdpane install claude-code|opencode|warp")
	}
	switch args[0] {
	case "claude-code":
		return installClaudeCode()
	case "opencode":
		return installOpenCode()
	case "warp":
		return installWarp()
	default:
		return fmt.Errorf("unknown target %q (claude-code, opencode, warp)", args[0])
	}
}

// installClaudeCode appends mdpane hooks to ~/.claude/settings.json,
// preserving everything already there.
func installClaudeCode() error {
	exe, err := os.Executable()
	if err != nil {
		exe = "mdpane"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude", "settings.json")

	settings := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return fmt.Errorf("%s exists but is not valid JSON: %w (fix it or add the hook manually)", path, err)
		}
	}

	hookEntry := func(matcher string) map[string]any {
		return map[string]any{
			"matcher": matcher,
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": exe + " hook claude-code",
				"timeout": 10,
			}},
		}
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	// Drop any previous mdpane entries, then add fresh ones (idempotent).
	post, _ := hooks["PostToolUse"].([]any)
	post = removeMdpaneEntries(post)
	post = append(post, hookEntry("Write|Edit"), hookEntry("ExitPlanMode"))
	hooks["PostToolUse"] = post
	settings["hooks"] = hooks

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(buf, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("installed Claude Code hooks in %s\n", path)
	fmt.Println("next markdown Write/Edit (including plan mode) will open in the mdpane pane")
	return nil
}

func removeMdpaneEntries(entries []any) []any {
	out := make([]any, 0, len(entries))
	for _, e := range entries {
		em, ok := e.(map[string]any)
		if ok {
			if hs, ok := em["hooks"].([]any); ok && len(hs) > 0 {
				if h0, ok := hs[0].(map[string]any); ok {
					if cmd, _ := h0["command"].(string); cmd != "" &&
						filepath.Base(firstWord(cmd)) == "mdpane" {
						continue
					}
				}
			}
		}
		out = append(out, e)
	}
	return out
}

func firstWord(s string) string {
	for i, r := range s {
		if r == ' ' {
			return s[:i]
		}
	}
	return s
}

// installOpenCode drops a TS plugin into ~/.config/opencode/plugin/.
func installOpenCode() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "opencode", "plugin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "mdpane.ts")
	if err := os.WriteFile(path, []byte(openCodePlugin), 0o644); err != nil {
		return err
	}
	fmt.Printf("installed OpenCode plugin at %s\n", path)
	return nil
}

const openCodePlugin = `// mdpane OpenCode plugin: live markdown preview of files the agent edits.
// Installed by 'mdpane install opencode'.
import type { Plugin } from "@opencode-ai/plugin"

export const MdpanePlugin: Plugin = async ({ $ }) => {
  return {
    event: async ({ event }) => {
      if (event.type !== "file.edited") return
      const file = (event as any).properties?.file
      if (!file || !/\.(md|markdown|mdown)$/i.test(file)) return
      // Fire-and-forget; never block or fail the agent loop.
      $` + "`mdpane open ${file}`" + `.quiet().nothrow().catch(() => {})
    },
  }
}
`

// installWarp writes the tab config used as Warp's split-pane substitute.
func installWarp() error {
	path := spawn.WarpTabConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(spawn.WarpTabConfig), 0o644); err != nil {
		return err
	}
	fmt.Printf("installed Warp tab config at %s\n", path)
	fmt.Println(`open it with: open "warp://tab_config/mdpane"  (mdpane open does this automatically)`)
	return nil
}
