// Transcript scrolling.
//
// scroll is the first visible line, or followTail while the view sticks
// to the latest line. Paging up sets a line; landing on the tail follows
// again, so the next output sticks to the bottom. The offset is a wrapped
// line, not a source message, so a resize can drift it.
package cli

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	pageOverlap = 1
	wheelRows   = 1
	// followTail sticks the transcript to its latest line.
	followTail = -1
)

func (m chatModel) onWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseWheelUp:
		return m.scrollRows(-wheelRows), nil
	case tea.MouseWheelDown:
		return m.scrollRows(wheelRows), nil
	default:
		return m, nil
	}
}

func (m chatModel) transcriptRows() int {
	input, _, _ := m.inputBlock()
	return roomFor(m.height, joinChrome(m.chromeAbove(), input, m.footer()))
}

// transcriptWindow is the visible slice of the transcript. Short history
// stays at the top of that slice; the spare rows sit above the composer.
func (m chatModel) transcriptWindow(room int) []string {
	if room <= 0 {
		return nil
	}
	lines := m.historyLines()
	start := m.viewTop(len(lines), room)
	end := start + room
	if end > len(lines) {
		end = len(lines)
	}
	window := make([]string, 0, room)
	if start < end {
		window = append(window, lines[start:end]...)
	}
	for len(window) < room {
		window = append(window, "")
	}
	return window
}

func (m chatModel) viewTop(n, room int) int {
	maxStart := max(n-room, 0)
	if m.scroll < 0 {
		return maxStart
	}
	return min(m.scroll, maxStart)
}

// scrollRows moves the transcript. Negative rows go toward older lines.
func (m chatModel) scrollRows(rows int) chatModel {
	if rows == 0 || !m.ready {
		return m
	}
	n := len(m.historyLines())
	room := m.transcriptRows()
	maxStart := n - room
	if maxStart < 1 {
		m.scroll = followTail
		return m
	}
	start := maxStart
	if m.scroll >= 0 {
		start = min(max(m.scroll, 0), maxStart)
	}
	next := start + rows
	if next >= maxStart {
		m.scroll = followTail
		return m
	}
	m.scroll = max(next, 0)
	return m
}

// pageRows is one viewport minus a one-line overlap, matching Codex.
func (m chatModel) pageRows() int {
	page := m.transcriptRows() - pageOverlap
	if page < 1 {
		return 1
	}
	return page
}

func (m chatModel) canReturn() bool {
	if m.scroll < 0 || !m.ready {
		return false
	}
	n := len(m.historyLines())
	room := m.transcriptRows()
	maxStart := n - room
	return maxStart > 0 && m.scroll < maxStart
}

func (m chatModel) transcript() string {
	return strings.Join(m.historyParts(false), "\n\n")
}

func (m chatModel) historyLines() []string {
	text := strings.Join(m.historyParts(true), "\n\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func (m chatModel) historyParts(includeLive bool) []string {
	parts := make([]string, 0, len(m.blocks)+2)
	if m.ready {
		if opening := m.opening(); opening != "" {
			parts = append(parts, opening)
		}
	}
	for _, b := range m.blocks {
		parts = append(parts, m.renderBlock(b))
	}
	if includeLive && strings.TrimSpace(m.live) != "" {
		parts = append(parts, m.paintAssistant(m.live))
	}
	return parts
}
