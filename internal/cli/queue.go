package cli

import (
	tea "charm.land/bubbletea/v2"
)

// queueText keeps one follow-up while a turn is running.
// Enter must not drop the draft. A second Enter extends that same message.
// /exit, exit, quit, /reset, and /compact are commands, not text to send later.
func (m *chatModel) queueText(text string) tea.Cmd {
	switch sessionCommand(text) {
	case cmdEmpty:
		return nil
	case cmdQuit:
		m.session.Abort()
		m.quitting = true
		return tea.Quit
	case cmdReset:
		m.addBlock(kindInfo, "still working — /reset waits")
		return nil
	case cmdCompact:
		m.addBlock(kindInfo, "still working — /compact waits")
		return nil
	}
	if m.queued == "" {
		m.queued = text
	} else {
		m.queued += "\n" + text
	}
	return nil
}

// takeQueue runs after AgentEnd. A finished turn sends the follow-up.
// An interrupt, an error, or a stop key that landed as the turn ended
// puts it back so the user can edit it first.
func (m *chatModel) takeQueue(err error) tea.Cmd {
	stop := m.stopping
	m.stopping = false
	if m.queued == "" {
		return nil
	}
	text := m.queued
	m.queued = ""
	if err != nil || stop {
		m.restoreQueued(text)
		return nil
	}
	return m.submit(text)
}

func (m *chatModel) restoreQueued(text string) {
	if cur := m.comp.text(); cur != "" {
		text += "\n" + cur
	}
	m.comp.set(text)
	m.noteTyped()
}

func (m *chatModel) queuedLine() string {
	if m.queued == "" {
		return ""
	}
	return fitLine(dimStyle.Render("↳ queued: "+oneLine(m.queued)), m.width)
}
