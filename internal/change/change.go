// Package change computes which lines of a fresh render differ from the
// previous render, so the UI can highlight exactly what the agent just wrote.
//
// Strategy: strip ANSI from both renders and line-diff the plain text
// (rendered output is hostile to direct diffing: escape codes pollute
// equality and rewrapping shifts content). udiff.Lines returns edits
// expanded to whole-line boundaries in the OLD text; we replay the edits to
// find the corresponding line ranges in the NEW text.
package change

import (
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/charmbracelet/x/ansi"
)

// Lines returns the set of line indices (0-based) in newRendered that are
// new or modified relative to oldRendered. A pure deletion marks the line
// that now sits at the deletion point, so removals remain visible.
func Lines(oldRendered, newRendered string) map[int]struct{} {
	changed := make(map[int]struct{})
	oldPlain := ansi.Strip(oldRendered)
	newPlain := ansi.Strip(newRendered)
	if oldPlain == newPlain {
		return changed
	}

	edits := udiff.Lines(oldPlain, newPlain)
	newLineCount := strings.Count(newPlain, "\n") + 1

	// Replay edits (sorted by Start, non-overlapping) tracking the offset
	// delta between old and new coordinates.
	delta := 0
	for _, e := range edits {
		newStart := e.Start + delta
		newEnd := newStart + len(e.New)
		delta += len(e.New) - (e.End - e.Start)

		startLine := lineAt(newPlain, newStart)
		var endLine int
		if len(e.New) == 0 {
			endLine = startLine // deletion: mark the survivor line
		} else {
			endLine = lineAt(newPlain, max(newStart, newEnd-1))
		}
		for i := startLine; i <= endLine && i < newLineCount; i++ {
			changed[i] = struct{}{}
		}
	}
	return changed
}

// IsAppend reports whether newPlainPrefix grew strictly by appending, which
// lets the UI keep highlights confined to the tail.
func IsAppend(oldRendered, newRendered string) bool {
	oldPlain := ansi.Strip(oldRendered)
	newPlain := ansi.Strip(newRendered)
	return len(newPlain) > len(oldPlain) && strings.HasPrefix(newPlain, oldPlain)
}

func lineAt(s string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(s) {
		offset = len(s)
	}
	return strings.Count(s[:offset], "\n")
}
