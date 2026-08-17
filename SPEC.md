# mdpane — Build Spec v1 (research-verified, Aug 2026)

A live terminal markdown pane for AI coding agents. It watches the markdown your
agent keeps rewriting (plans, specs, READMEs), re-renders it styled on every
write, highlights what changed, and auto-follows the most recently written file.
Agent-agnostic by design: the core needs zero integration; Claude Code and
OpenCode get one-line adapters.

Single Go binary. MIT. Repo: github.com/rishabhky/mdpane.

## Positioning (verified against the landscape)

What is table stakes in 2026: live reload. glow has shipped watch/auto-reload
since v2.1.0 (Feb 2025) — do NOT pitch against "glow can't reload".

What nothing does (verified across brew/crates/npm/GitHub, Aug 2026):
1. **Change highlighting** on re-render (what did the agent just edit?).
2. **Follow-most-recent**: automatically switch to whichever markdown file was
   written last.
3. **Agent integration**: hooks/plugins that pop the pane open and drive it.
4. Daemonless attach model that works even in terminals with no split API.

Closest competitors: cmux-markdown (locked inside the cmux desktop app),
three browser-based Claude plan viewers (<10 stars each). The canonical
awesome-claude-code list has an empty slot for this category.

README foil (cite it): glow discussion #426 — a user wires up
`ls docs/*.md | entr -r glow README.md` and complains it "clears the terminal
and re-renders from scratch, creating a jarring experience."

## Architecture

**The viewer is the server.** No background daemon to manage:

- `mdpane attach` runs the TUI *and* listens on a unix socket
  (`/tmp/mdpane-$UID.sock`; keep the path short for the 104-byte sun_path
  limit). It renders whatever file it is currently pointed at.
- `mdpane open FILE` is a tiny idempotent client: dial the socket; if a viewer
  answers, send `{"cmd":"open","path":...}` (newline-delimited JSON) and exit.
  If nobody is listening, bootstrap a pane for the current terminal (matrix
  below) running `mdpane attach --open FILE`, then exit. Guard the
  dial-then-listen race with a gofrs/flock lock file (auto-released on death,
  no stale-PID problem). Prior art for this exact pattern: lf -remote, gopls
  daemon.
- `mdpane [FILE]` with no subcommand = view one file directly (glow-parity
  entry, still watches + highlights). `mdpane follow [DIR...]` = attach without
  a socket-driven target: watch dirs, follow the most recently written *.md.

Commands:

```
mdpane [FILE]                    view one file (watch + highlight)
mdpane follow [DIR...]           follow most-recent .md (default: cwd + ~/.claude/plans)
mdpane attach                    pane mode: TUI + socket server (used by open/bootstrap)
mdpane open FILE                 idempotent: point the pane at FILE, spawning it if needed
mdpane install claude-code       write hook config (or print plugin instructions)
mdpane install opencode          drop the OpenCode plugin into ~/.config/opencode/plugin
mdpane install warp              write ~/.warp/tab_configs/mdpane.toml
```

## Stack (verified versions — the Charm ecosystem went v2 in early 2026)

| Concern | Choice |
|---|---|
| Rendering | `charm.land/glamour/v2` (v2.0.1) — NOT the old github.com/charmbracelet path |
| TUI | `charm.land/bubbletea/v2` + `charm.land/bubbles/v2/viewport` (damage-tracked renderer, ~10x faster) |
| Styling | `charm.land/lipgloss/v2` |
| Watching | `fsnotify` v1.9.0 |
| Diff | `aymanbagabas/go-udiff` (gopls' diff). NOT sergi/go-diff (known correctness bug — go-git is pinned to a 2019 version because of it) |
| Locking | `gofrs/flock` |
| Release | goreleaser v2: `homebrew_casks` (the `brews` section is deprecated), scoop, nfpm; aqua-registry PR (also enables mise) |

Notes that shape the code:
- glamour is whole-document only (Write buffers, Close renders — verified in
  source). Design = full re-render per change + diff. Tens of ms for a few
  thousand lines; cache chroma-highlighted code blocks by content hash if
  profiling demands.
- glamour v2 removed WithAutoStyle; pick dark default, `--style` flag.
  Word-wrap width is fixed at construction: recreate the renderer on resize.
- viewport v2 has `StyleLineFunc` (per-line background = change highlight),
  `LeftGutterFunc` (change marker column `▎`), `AtBottom()`/`GotoBottom()`,
  built-in mouse wheel. These are exactly our primitives.

## Core behaviors

**Watching.** Always watch the parent directory, never the file: agents and
editors write temp-then-rename, which silently kills a file watch (fsnotify
#372). Filter events by name; treat Create/Rename of the target as change;
debounce 150ms; file shrink = full reload, pure append = tail highlight only.

**Change highlighting.** Two-render line diff: keep the previous render,
`x/ansi.Strip` both renders, line-diff with go-udiff, paint changed rendered
lines via StyleLineFunc (subtle background tint) + gutter marker. Fade the tint
after ~4s via tea.Tick. (Block-level AST mapping via goldmark is the v2
refinement if line diff proves noisy — not v1.)

**Follow semantics (`less +F` UX).** `following=true` → GotoBottom on update.
Any upward scroll/key sets `following = vp.AtBottom()` after the update, so
scrolling to bottom naturally re-engages; `G` forces it. Status bar always
shows state: `FOLLOWING` vs `scroll 43% · G to follow`.

**Follow-most-recent.** In follow/attach mode, watch the configured dirs
(recursive, with ignore globs: node_modules, .git, vendor). On any *.md write,
switch the view to that file (with a 1-line "switched to X" flash). A recency
list overlay (`tab`) lets the user jump among the last N files. `p` pins the
current file (stops auto-switching until unpinned).

**Scroll preservation on re-render.** Capture AtBottom + YOffset before
SetContent; restore offset (or GotoBottom if following). Never yank the user.

## Terminal bootstrap matrix (for `mdpane open`)

Detect via env: `$TMUX`, `$ZELLIJ`, `$KITTY_LISTEN_ON`, `$WEZTERM_PANE`,
`$TERM_PROGRAM` (WarpTerminal / iTerm.app / ghostty). Hooks run as children of
the agent CLI, so they inherit the right pane's env.

| Terminal | Bootstrap |
|---|---|
| tmux | `tmux split-window -h -l 45% 'mdpane attach --open FILE'` |
| WezTerm | `wezterm cli split-pane --right --percent 40 -- mdpane attach ...` (zero config) |
| kitty | `kitten @ launch --type=window --location=vsplit --var mdpane=1 -- ...` (needs allow_remote_control; detect & degrade) |
| iTerm2 | osascript `split vertically with default profile command "..."` |
| Zellij | `zellij action new-pane --direction right --name mdpane -- ...` |
| **Warp** | no split API (issue #1550 open since 2022). Ship a Tab Config: `open 'warp://tab_config/mdpane'` opens a preconfigured shell+preview tab once; after that the attach socket handles everything |
| Ghostty / unknown | print: `Run 'mdpane attach' in another pane` (one-time; socket takes over after) |

The attach-socket model means bootstrap runs at most once per session in every
terminal — pane reuse is free everywhere because the pane never restarts, only
the viewer's target file changes.

## Agent adapters (Layer 1 — all optional; Layer 0 is plain file watching)

**Claude Code** (verified hook facts):
- PostToolUse fires for Write|Edit with `tool_input.file_path` in stdin JSON —
  including plan files, which are written incrementally to `~/.claude/plans/`
  (configurable via `plansDirectory`) during plan mode. So the pane shows the
  plan being written live, before approval.
- `ExitPlanMode` is a matchable tool = "plan finalized" signal
  (`tool_response` carries the plan path). Known bug: its hooks can run with
  cwd=~; always use absolute paths from the JSON, never cwd.
- Ship both: (a) a Claude Code **plugin** in-repo (`.claude-plugin/` +
  `hooks/hooks.json`) installable via `/plugin marketplace add
  rishabhky/mdpane`, and (b) `mdpane install claude-code` which appends the
  hook to `~/.claude/settings.json` for non-plugin users. Hook body: match
  `Write|Edit`, filter `*.md`, fire-and-forget `mdpane open "$path" &` with a
  short timeout (hooks block the agent loop).

**OpenCode** (verified): TS plugin, ~15 lines — subscribe to `file.edited`
(and `tool.execute.after`), shell out to `mdpane open <path>` for `*.md`.
Ships in-repo under `integrations/opencode/`; `mdpane install opencode` copies
it into place.

**Everything else** (Codex CLI, Cursor CLI, Gemini CLI, humans with vim):
covered by Layer 0 — `mdpane follow .` needs no integration at all. Adapters
are additive convenience, so the tool is generic by construction.

## Repo layout

```
mdpane/
  main.go                     # cobra-less: stdlib flag + subcommand dispatch (keep deps lean)
  internal/ui/                # bubbletea model: viewport, status bar, recency overlay
  internal/render/            # glamour wrapper, width/theme mgmt, render cache
  internal/watch/             # dir watcher, debounce, most-recent tracker
  internal/change/            # strip+diff+line-range mapping, fade state
  internal/ipc/               # socket protocol (ndjson), flock, open client
  internal/spawn/             # terminal detection + split bootstrap matrix
  integrations/claude-code/   # plugin: .claude-plugin/plugin.json, hooks/hooks.json
  integrations/opencode/      # plugin.ts
  integrations/warp/          # mdpane.toml tab config
  demo/                       # vhs tape for the README GIF
  .goreleaser.yaml  .github/workflows/{ci,release}.yml
```

## Milestones

**M0 — scaffold.** go mod (charm.land v2 deps), CI (build + test + lint,
ubuntu/macos), goreleaser skeleton, LICENSE/README stub. ✅ CI green.

**M1 — the viewer.** `mdpane FILE`: glamour render in a viewport, watch parent
dir, debounce, re-render in place with scroll preservation, resize handling,
`q`/arrows/mouse. ✅ Edit the file in another pane → updates in place, no
flicker, no scroll jump. This alone already beats the entr+glow hack.

**M2 — change highlighting + follow.** Two-render diff, StyleLineFunc tint +
gutter markers, 4s fade, less+F follow semantics with status-bar state.
✅ Append → tail highlighted and followed; edit mid-file → that region
highlighted, scroll stays put; scroll up → following disengages visibly.

**M3 — follow-most-recent.** Multi-dir recursive watch with ignore globs,
auto-switch on newest *.md, recency overlay (tab), pin (p). Default dirs:
cwd + ~/.claude/plans. ✅ Two files written alternately by a script → pane
switches; pin holds; tab lists both.

**M4 — attach/open IPC + terminal bootstrap.** Socket server inside attach
mode, ndjson protocol, flock guard, `mdpane open` client, spawn matrix (tmux,
WezTerm, kitty, iTerm2, Zellij, Warp tab-config, fallback message).
✅ `mdpane open x.md` from a bare shell: first call opens a pane, second call
retargets it, in tmux and in Warp (via tab config) on this machine.

**M5 — agent adapters.** Claude Code plugin + settings installer, OpenCode
plugin + installer, docs for both. ✅ Live demo on this machine: Claude Code
plan mode writes a plan → pane opens/updates live during planning; OpenCode
edit does the same.

**M6 — polish.** Config file (~/.config/mdpane/config.toml: theme, width,
dirs, ignores, fade, split direction/size), `--style` flag, `?` keybinding
help, large-doc performance pass (render cache), golden-file render tests,
teatest-based TUI tests where cheap.

**M7 — ship it.** goreleaser release: darwin/linux/windows × amd64/arm64,
homebrew cask via rishabhky/homebrew-tap, scoop manifest, deb/rpm; aqua
registry PR; `go install` path kept working. README: vhs-generated GIF of the
killer demo (Claude Code writing a plan, pane rendering it live with
highlights), the entr+glow foil, install matrix, integration docs. Submit to
awesome-claude-code list + Terminal Trove.

## Non-goals for v1 (explicit)

- Inline images (glow doesn't either; alt-text links via glamour default).
- Streaming/partial-markdown rendering (glamour is whole-doc; re-render+diff
  is the design). Candidate for v2.
- Windows split bootstrap (binary runs; Windows Terminal panes later).
- Browser fallback, themes marketplace, editing.

## Success criteria

It replaces the DIY hack for real users: `brew install rishabhky/tap/mdpane`,
`/plugin install mdpane`, and the next time Claude Code plans something, a
styled, change-highlighted plan appears beside it — including on Warp, the
author's own terminal, where no split API exists.
