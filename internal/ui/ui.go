// Package ui is the mdpane viewer: a viewport over glamour-rendered
// markdown with change highlighting, less+F follow semantics, a recency
// switcher, and (in attach mode) a socket that retargets the pane.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rishabhky/mdpane/internal/change"
	"github.com/rishabhky/mdpane/internal/render"
	"github.com/rishabhky/mdpane/internal/watch"
)

const (
	fadeAfter    = 4 * time.Second
	flashAfter   = 2500 * time.Millisecond
	maxRecent    = 15
	gutterActive = "▎ "
	gutterIdle   = "  "
)

var (
	gutterMark   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7ee787"))
	barStyle     = lipgloss.NewStyle().Background(lipgloss.Color("#1a1b26")).Foreground(lipgloss.Color("#a9b1d6")).Padding(0, 1)
	barAccent    = lipgloss.NewStyle().Background(lipgloss.Color("#1a1b26")).Foreground(lipgloss.Color("#7ee787")).Bold(true)
	barDim       = lipgloss.NewStyle().Background(lipgloss.Color("#1a1b26")).Foreground(lipgloss.Color("#565f89"))
	barWarn      = lipgloss.NewStyle().Background(lipgloss.Color("#1a1b26")).Foreground(lipgloss.Color("#e0af68")).Bold(true)
	overlayTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7ee787"))
	overlaySel   = lipgloss.NewStyle().Background(lipgloss.Color("#283457")).Bold(true)
	overlayDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
)

// Config wires a viewer.
type Config struct {
	InitialFile  string
	Dirs         []string // watched directories
	FollowNewest bool     // switch to the most recently written markdown file
	Style        string
	SocketNote   string          // status hint, e.g. "attached" or "no socket"
	Opens        <-chan string   // socket open requests (may be nil)
	Quit         <-chan struct{} // closed when another viewer takes over (may be nil)
}

type (
	fileEventMsg string
	openMsg      string
	takeoverMsg  struct{}
	tickMsg      time.Time
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayRecent
	overlayHelp
)

type Model struct {
	cfg      Config
	renderer *render.Renderer
	watcher  *watch.Watcher

	vp        viewport.Model
	width     int
	height    int
	ready     bool
	following bool
	pinned    bool

	file         string
	rendered     string
	changed      map[int]time.Time
	ticking      bool
	recent       []string
	overlay      overlayKind
	overlayIndex int
	flash        string
	flashUntil   time.Time
	errNote      string
}

func NewModel(cfg Config, r *render.Renderer, w *watch.Watcher) *Model {
	m := &Model{
		cfg:       cfg,
		renderer:  r,
		watcher:   w,
		following: true,
		changed:   map[int]time.Time{},
		file:      cfg.InitialFile,
	}
	m.vp = viewport.New()
	m.vp.MouseWheelEnabled = true
	m.vp.FillHeight = true
	// Changed lines get a gutter bar only (GitHub/VS Code style). No
	// background tint: glamour pads lines to the wrap width, so painting
	// line backgrounds turns every change into a full-width slab.
	m.vp.LeftGutterFunc = func(gc viewport.GutterContext) string {
		if _, ok := m.changed[gc.Index]; ok {
			return gutterMark.Render(gutterActive)
		}
		return gutterIdle
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.waitWatch()}
	if m.cfg.Opens != nil {
		cmds = append(cmds, m.waitOpen())
	}
	if m.cfg.Quit != nil {
		quit := m.cfg.Quit
		cmds = append(cmds, func() tea.Msg {
			<-quit
			return takeoverMsg{}
		})
	}
	return tea.Batch(cmds...)
}

func (m *Model) waitWatch() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.watcher.Events()
		if !ok {
			return nil
		}
		return fileEventMsg(ev.Path)
	}
}

func (m *Model) waitOpen() tea.Cmd {
	return func() tea.Msg {
		p, ok := <-m.cfg.Opens
		if !ok {
			return nil
		}
		return openMsg(p)
	}
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 20), max(msg.Height, 4)
		m.vp.SetWidth(m.width)
		m.vp.SetHeight(m.height - 1) // status bar
		_ = m.renderer.SetWidth(msg.Width - 4)
		first := !m.ready
		m.ready = true
		if m.file != "" {
			m.reload(m.file, !first) // resize is a reload without highlights on first paint
			if first {
				m.clearChanged()
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		m.following = m.vp.AtBottom()
		return m, cmd

	case fileEventMsg:
		path := string(msg)
		m.touchRecent(path)
		switch {
		case path == m.file:
			m.reload(path, true)
		case m.cfg.FollowNewest && !m.pinned:
			m.switchTo(path, "→ "+prettyPath(path))
		}
		return m, tea.Batch(m.waitWatch(), m.ensureTick())

	case takeoverMsg:
		return m, tea.Quit

	case openMsg:
		path := string(msg)
		m.pinned = false
		m.touchRecent(path)
		m.switchTo(path, "opened "+prettyPath(path))
		return m, tea.Batch(m.waitOpen(), m.ensureTick())

	case tickMsg:
		now := time.Time(msg)
		for i, t := range m.changed {
			if now.Sub(t) > fadeAfter {
				delete(m.changed, i)
			}
		}
		if m.flash != "" && now.After(m.flashUntil) {
			m.flash = ""
		}
		if len(m.changed) == 0 && m.flash == "" {
			m.ticking = false
			return m, nil
		}
		return m, tick()
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.overlay != overlayNone {
		switch key {
		case "esc", "q", "tab", "?":
			m.overlay = overlayNone
		case "j", "down":
			if m.overlayIndex < len(m.recent)-1 {
				m.overlayIndex++
			}
		case "k", "up":
			if m.overlayIndex > 0 {
				m.overlayIndex--
			}
		case "enter":
			if m.overlay == overlayRecent && m.overlayIndex < len(m.recent) {
				m.pinned = false
				m.switchTo(m.recent[m.overlayIndex], "opened "+prettyPath(m.recent[m.overlayIndex]))
			}
			m.overlay = overlayNone
		case "ctrl+c":
			return m, tea.Quit
		}
		return m, m.ensureTick()
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "g", "home":
		m.vp.GotoTop()
		m.following = false
	case "G", "end":
		m.vp.GotoBottom()
		m.following = true
	case "f":
		m.vp.GotoBottom()
		m.following = true
	case "p":
		if m.cfg.FollowNewest {
			m.pinned = !m.pinned
			if m.pinned {
				m.setFlash("pinned to " + prettyPath(m.file))
			} else {
				m.setFlash("unpinned")
			}
		}
	case "r":
		if m.file != "" {
			m.reload(m.file, false)
			m.clearChanged()
			m.setFlash("reloaded")
		}
	case "tab":
		if len(m.recent) > 0 {
			m.overlay = overlayRecent
			m.overlayIndex = 0
		}
	case "?":
		m.overlay = overlayHelp
	default:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		m.following = m.vp.AtBottom()
		return m, cmd
	}
	return m, m.ensureTick()
}

// reload re-reads and re-renders the current file. highlight=true diffs
// against the previous render and paints the changes.
func (m *Model) reload(path string, highlight bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		m.errNote = fmt.Sprintf("read error: %v", err)
		return
	}
	m.errNote = ""
	rendered, err := m.renderer.Render(string(raw))
	if err != nil {
		m.errNote = fmt.Sprintf("render error: %v", err)
		return
	}

	atBottom := m.vp.AtBottom()
	offset := m.vp.YOffset()

	if highlight && m.rendered != "" {
		now := time.Now()
		fresh := change.Lines(m.rendered, rendered)
		m.changed = map[int]time.Time{}
		for i := range fresh {
			m.changed[i] = now
		}
	}
	m.rendered = rendered
	m.vp.SetContent(rendered)

	if m.following || atBottom {
		m.vp.GotoBottom()
		m.following = true
	} else {
		m.vp.SetYOffset(offset)
	}
}

func (m *Model) switchTo(path, flash string) {
	m.file = path
	m.rendered = ""
	m.clearChanged()
	m.reload(path, false)
	m.setFlash(flash)
	m.following = true
	m.vp.GotoBottom()
}

func (m *Model) clearChanged() { m.changed = map[int]time.Time{} }

func (m *Model) setFlash(s string) {
	m.flash = s
	m.flashUntil = time.Now().Add(flashAfter)
}

func (m *Model) ensureTick() tea.Cmd {
	if m.ticking || (len(m.changed) == 0 && m.flash == "") {
		return nil
	}
	m.ticking = true
	return tick()
}

func (m *Model) touchRecent(path string) {
	out := []string{path}
	for _, p := range m.recent {
		if p != path {
			out = append(out, p)
		}
	}
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	m.recent = out
}

func (m *Model) View() tea.View {
	var body string
	switch m.overlay {
	case overlayRecent:
		body = m.recentOverlay()
	case overlayHelp:
		body = helpOverlay(m.width, m.height-1)
	default:
		body = m.vp.View()
	}
	v := tea.NewView(body + "\n" + m.statusBar())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) statusBar() string {
	name := "(no file)"
	if m.file != "" {
		name = prettyPath(m.file)
	}
	left := barAccent.Render(" "+name) + barStyle.Render("")

	var mid string
	switch {
	case m.errNote != "":
		mid = barWarn.Render(m.errNote)
	case m.flash != "":
		mid = barStyle.Render(m.flash)
	}

	state := fmt.Sprintf("%3.0f%%", m.vp.ScrollPercent()*100)
	if m.following {
		state = "FOLLOW"
	}
	if m.pinned {
		state += " · PIN"
	}
	right := barStyle.Render(state) + barDim.Render("tab:files  ?:help ")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	filler := barStyle.Render(padSpaces(gap))
	return left + mid + filler + right
}

func (m *Model) recentOverlay() string {
	lines := []string{"", "  " + overlayTitle.Render("Recent files") + overlayDim.Render("   enter: open · esc: close"), ""}
	for i, p := range m.recent {
		row := "   " + prettyPath(p)
		if i == m.overlayIndex {
			row = overlaySel.Render(" » " + prettyPath(p) + " ")
		}
		lines = append(lines, row)
	}
	return padLines(lines, m.height-1)
}

func helpOverlay(w, h int) string {
	lines := []string{
		"",
		"  " + overlayTitle.Render("mdpane keys"),
		"",
		"   j/k, arrows, wheel   scroll",
		"   g / G                top / bottom",
		"   f                    follow (auto-scroll on change)",
		"   p                    pin current file (stop auto-switching)",
		"   tab                  recent files",
		"   r                    force reload",
		"   q                    quit",
		"",
		"  " + overlayDim.Render("changed lines glow, then fade; the pane follows the"),
		"  " + overlayDim.Render("most recently written markdown file unless pinned."),
	}
	_ = w
	return padLines(lines, h)
}

func padLines(lines []string, height int) string {
	out := ""
	for i := 0; i < height; i++ {
		if i < len(lines) {
			out += lines[i]
		}
		if i < height-1 {
			out += "\n"
		}
	}
	return out
}

func padSpaces(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

func prettyPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, p); err == nil && !filepath.IsAbs(rel) && rel != ".." && !hasDotDotPrefix(rel) {
			return "~/" + rel
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && !hasDotDotPrefix(rel) {
			return rel
		}
	}
	return p
}

func hasDotDotPrefix(p string) bool {
	return p == ".." || (len(p) > 2 && p[:3] == "../")
}
