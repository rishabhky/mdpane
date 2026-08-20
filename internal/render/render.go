// Package render wraps glamour: markdown in, styled ANSI out, at a width.
//
// glamour renders whole documents only (its streaming Writer just buffers),
// so mdpane's model is full re-render per change + diff. Width is fixed at
// renderer construction, so we rebuild the renderer on resize.
package render

import (
	"sync"

	glamour "charm.land/glamour/v2"
)

const (
	DefaultStyle = "dark"
	// MaxWidth caps the rendered line width: prose wider than ~120 columns
	// is harder to read, not easier.
	MaxWidth = 120
	MinWidth = 20
)

type Renderer struct {
	mu    sync.Mutex
	style string
	width int
	tr    *glamour.TermRenderer
}

func New(style string, width int) (*Renderer, error) {
	r := &Renderer{style: style}
	if r.style == "" {
		r.style = DefaultStyle
	}
	if err := r.SetWidth(width); err != nil {
		return nil, err
	}
	return r, nil
}

func clampWidth(w int) int {
	if w < MinWidth {
		return MinWidth
	}
	if w > MaxWidth {
		return MaxWidth
	}
	return w
}

// SetWidth rebuilds the underlying renderer if the width changed.
func (r *Renderer) SetWidth(w int) error {
	w = clampWidth(w)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tr != nil && w == r.width {
		return nil
	}
	tr, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(r.style),
		glamour.WithWordWrap(w),
		glamour.WithTableWrap(true),
		glamour.WithEmoji(),
	)
	if err != nil {
		return err
	}
	r.width, r.tr = w, tr
	return nil
}

func (r *Renderer) Width() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.width
}

// Render converts markdown to styled ANSI text.
func (r *Renderer) Render(markdown string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tr.Render(markdown)
}
