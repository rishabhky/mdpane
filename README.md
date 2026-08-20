# mdpane

A live terminal markdown pane for AI coding agents.

Your agent keeps rewriting markdown at you: plans, specs, READMEs. Reading raw
markdown in a chat scrollback is miserable, and the classic workaround
(`ls *.md | entr -r glow ...`) clears the screen and re-renders from scratch on
every save. mdpane is a small Go binary that does this properly:

- **Live re-render, in place.** Watches the file and re-renders on every write
  with no flicker and no scroll jump. Handles the temp-then-rename saves agents
  and editors actually do.
- **Change highlighting.** The lines your agent just changed glow, then fade.
  You see *what changed*, not just that something did.
- **Follows the newest file.** In follow mode the pane automatically switches
  to whichever markdown file was written most recently. The agent moves on to
  the README? So does the pane. Pin with `p` when you want to stay put.
- **Follow-mode scrolling like `less +F`.** Auto-scrolls on growth; scroll up
  and it stops; hit `G` to re-engage.
- **A pane you can drive.** `mdpane open FILE` retargets the running pane over
  a unix socket, or spawns one — real splits in tmux, WezTerm, kitty, iTerm2,
  Zellij, and a preconfigured tab in Warp.
- **Agent-agnostic.** The core is plain file watching, so it works with any
  agent (or human) that writes markdown. Claude Code and OpenCode get one-line
  integrations that open the pane automatically.

## Install

```sh
go install github.com/rishabhky/mdpane@latest
# or grab a binary from Releases; brew tap coming with the first release
```

## Use

```sh
mdpane README.md          # view one file, live
mdpane follow             # follow the newest .md under . and ~/.claude/plans
mdpane attach             # pane mode: also listens for 'mdpane open'
mdpane open notes.md      # retarget the pane (spawns one if none is running)
```

Keys: `j/k` scroll · `g/G` top/bottom · `f` follow · `p` pin · `tab` recent
files · `r` reload · `?` help · `q` quit.

## With Claude Code

```sh
mdpane install claude-code
```

That adds hooks to `~/.claude/settings.json`: every markdown Write/Edit —
including **plan files as Claude writes them during plan mode** — pops up in
the pane, styled, with the fresh edits highlighted. Prefer plugins? The same
integration ships as a Claude Code plugin:

```
/plugin marketplace add rishabhky/mdpane
/plugin install mdpane@mdpane
```

## With OpenCode

```sh
mdpane install opencode
```

Installs a tiny plugin that fires `mdpane open` whenever OpenCode edits a
markdown file.

## With any other agent

No integration needed — run `mdpane follow` in a side pane and it tracks
whatever gets written. That's the whole point.

## Terminals

`mdpane open` spawns its pane with a real split where the terminal allows it:
tmux, WezTerm, iTerm2, Zellij out of the box; kitty with
`allow_remote_control` enabled. Warp has no split API, so
`mdpane install warp` sets up a Tab Config that opens a shell + preview
layout with one command (`mdpane open` launches it automatically). Anywhere
else: run `mdpane attach` in any pane once, and the socket takes it from
there.

## How it works

One binary, no daemon: the viewer *is* the server. `mdpane attach` renders in
your terminal and listens on a unix socket; `mdpane open` is a client that
retargets it. Rendering is [glamour], watching is fsnotify on parent
directories (rename-proof), and change detection strips ANSI from consecutive
renders and line-diffs them ([go-udiff]) to paint highlights via the viewport's
per-line styling.

[glamour]: https://github.com/charmbracelet/glamour
[go-udiff]: https://github.com/aymanbagabas/go-udiff

## Development

```sh
go test -race ./...
go build -o bin/mdpane .
```

MIT.
