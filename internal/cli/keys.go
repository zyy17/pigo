// Key bindings for the chat.
//
// Enter sends. Shift+Enter inserts a newline when the terminal can tell
// the two apart. Bubble Tea v2 asks for that disambiguation (the Kitty
// keyboard protocol and xterm modifyOtherKeys) and turns it off on quit.
// Ctrl+J, Alt+Enter, and a trailing backslash still insert a newline on
// terminals that report Shift+Enter as a plain Enter.

package cli

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m chatModel) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.onInterrupt()
	case "ctrl+d":
		if !m.busy && strings.TrimSpace(m.comp.text()) == "" {
			m.quitting = true
			return m, tea.Quit
		}
	}
	if !msg.Mod.Contains(tea.ModAlt) {
		switch msg.Code {
		case tea.KeyPgUp:
			return m.scrollRows(-m.pageRows()), nil
		case tea.KeyPgDown:
			return m.scrollRows(m.pageRows()), nil
		}
	}
	if m.ask != nil {
		return m.answerConfirm(msg)
	}
	if m.busy {
		// Esc stops the turn. The working row is the hint for that key.
		// A confirm prompt owns Esc first and uses it to deny.
		if msg.Code == tea.KeyEsc && !msg.Mod.Contains(tea.ModAlt) && m.session != nil {
			m.session.Abort()
		}
		return m, nil
	}
	// Esc returns a paused transcript to the tail. The slash menu owns
	// Esc before this, and a running turn already used it to interrupt.
	if msg.Code == tea.KeyEsc && !msg.Mod.Contains(tea.ModAlt) && !m.menuOpen() && m.canReturn() {
		m.scroll = followTail
		return m, nil
	}
	return m.onComposer(msg)
}

func (m chatModel) onInterrupt() (tea.Model, tea.Cmd) {
	if m.ask != nil {
		m = m.finishConfirm(false)
	}
	if m.busy {
		if m.session != nil {
			m.session.Abort()
		}
		return m, nil
	}
	m.quitting = true
	return m, tea.Quit
}

func (m chatModel) onComposer(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.Code == tea.KeyEnter || msg.Code == tea.KeyKpEnter {
		if msg.Mod.Contains(tea.ModShift) || msg.Mod.Contains(tea.ModAlt) {
			return m.newline()
		}
		return m.onEnter()
	}
	switch msg.String() {
	case "ctrl+j":
		return m.newline()
	case "tab":
		if m.menuOpen() {
			m.completeSuggest()
		}
		return m, nil
	case "esc":
		m.suggestDismissed = true
		return m, nil
	case "up":
		if m.menuOpen() {
			m.moveSuggest(-1)
		}
		return m, nil
	case "down":
		if m.menuOpen() {
			m.moveSuggest(1)
		}
		return m, nil
	case "backspace":
		m.comp.backspace()
		m.noteTyped()
		return m, nil
	case "delete":
		m.comp.del()
		m.noteTyped()
		return m, nil
	case "left", "ctrl+b":
		m.comp.left()
		return m, nil
	case "right", "ctrl+f":
		m.comp.right()
		return m, nil
	case "home", "ctrl+a":
		m.comp.home()
		return m, nil
	case "end", "ctrl+e":
		m.comp.end()
		return m, nil
	case "ctrl+u":
		m.comp.clear()
		m.noteTyped()
		return m, nil
	case "ctrl+w":
		m.comp.wordBack()
		m.noteTyped()
		return m, nil
	case "space":
		m.comp.insert(" ")
		m.noteTyped()
		return m, nil
	default:
		if msg.Text != "" && !msg.Mod.Contains(tea.ModCtrl) {
			m.comp.insert(msg.Text)
			m.noteTyped()
		}
		return m, nil
	}
}

func (m chatModel) newline() (tea.Model, tea.Cmd) {
	m.comp.insert("\n")
	m.noteTyped()
	return m, nil
}

func (m chatModel) onEnter() (tea.Model, tea.Cmd) {
	if m.menuOpen() && partialCommand(m.comp.text(), m.matches()) {
		m.completeSuggest()
		return m, nil
	}
	if next, ok := continuedLine(m.comp.text()); ok {
		m.comp.set(next)
		m.noteTyped()
		return m, nil
	}
	return m.submit(strings.TrimSpace(m.comp.text()))
}

// continuedLine is the fallback for a terminal that cannot report
// Shift+Enter: a trailing backslash and Enter inserts a newline and drops
// that backslash. An even count is an escaped backslash, so the line
// submits as typed. Spaces after the slash cancel it.
func continuedLine(text string) (string, bool) {
	head := ""
	line := text
	if i := strings.LastIndex(text, "\n"); i >= 0 {
		head = text[:i+1]
		line = text[i+1:]
	}
	if strings.TrimRight(line, " \t") != line {
		return "", false
	}
	n := 0
	for strings.HasSuffix(line, `\`) {
		n++
		line = strings.TrimSuffix(line, `\`)
	}
	if n == 0 || n%2 == 0 {
		return "", false
	}
	return head + line + strings.Repeat(`\`, n-1) + "\n", true
}
