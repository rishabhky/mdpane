package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/rishabhky/mdpane/internal/watch"
)

// cmdHook handles agent hook invocations. Claude Code pipes a JSON event on
// stdin; we extract the file path and fire-and-forget an `open`. Hooks block
// the agent loop, so this must return fast and never fail the tool call.
func cmdHook(args []string) error {
	if len(args) < 1 || args[0] != "claude-code" {
		return fmt.Errorf("usage: mdpane hook claude-code")
	}
	var payload struct {
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			FilePath string `json:"file_path"`
		} `json:"tool_input"`
		ToolResponse json.RawMessage `json:"tool_response"`
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 10<<20))
	if err != nil || len(raw) == 0 {
		return nil // no payload: exit 0, never break the agent
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}

	path := payload.ToolInput.FilePath
	if payload.ToolName == "ExitPlanMode" && path == "" {
		// ExitPlanMode carries the plan path in tool_response.
		var resp struct {
			FilePath string `json:"filePath"`
		}
		if json.Unmarshal(payload.ToolResponse, &resp) == nil {
			path = resp.FilePath
		}
	}
	if path == "" || !watch.IsMarkdown(path) {
		return nil
	}
	// ExitPlanMode hooks may run with cwd=~; only absolute paths are safe.
	if !strings.HasPrefix(path, "/") {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		exe = "mdpane"
	}
	// Detach: the open (and possible pane spawn) must not block the agent.
	cmd := exec.Command(exe, "open", path)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = nil, nil, nil
	if err := cmd.Start(); err == nil {
		_ = cmd.Process.Release()
	}
	return nil
}
