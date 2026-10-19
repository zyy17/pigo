// Transcript scrolling.
//
// follow pins the view to the latest line. top is the first visible line
// while the user is reading older history. Paging up clears follow; landing
// on the tail sets it again, so the next output sticks to the bottom.
// The offset is a wrapped line, not a source message, so a resize can
// drift it. The next paint rewraps from the source text.
package cli

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	pageOverlap = 1
	wheelRows   = 1
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
	maxStart := n - room
	if maxStart < 0 {
		maxStart = 0
	}
	if m.follow || m.top >= maxStart {
		return maxStart
	}
	if m.top < 0 {
		return 0
	}
	return m.top
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
		m.follow = true
		m.top = 0
		return m
	}
	start := maxStart
	if !m.follow {
		start = m.top
		if start < 0 {
			start = 0
		}
		if start > maxStart {
			start = maxStart
		}
	}
	next := start + rows
	if next < 0 {
		next = 0
	}
	if next >= maxStart {
		m.follow = true
		m.top = 0
		return m
	}
	m.follow = false
	m.top = next
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
	if m.follow || !m.ready {
		return false
	}
	n := len(m.historyLines())
	room := m.transcriptRows()
	maxStart := n - room
	return maxStart > 0 && m.top < maxStart
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
