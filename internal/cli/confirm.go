package cli

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// confirmReq is one dangerous command waiting for the user.
type confirmReq struct {
	command string
	reason  string
	reply   chan bool
}

type confirmMsg struct {
	req *confirmReq
}

func (m chatModel) answerConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.finishConfirm(true), nil
	case tea.KeyEsc:
		return m.finishConfirm(false), nil
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'y', 'Y':
				return m.finishConfirm(true), nil
			case 'n', 'N':
				return m.finishConfirm(false), nil
			}
		}
	}
	return m, nil
}

func (m chatModel) finishConfirm(allow bool) chatModel {
	if m.ask == nil {
		return m
	}
	select {
	case m.ask.reply <- allow:
	default:
	}
	m.ask = nil
	return m
}

// confirmView replaces the working row while a dangerous command is waiting.
// Esc denies that command. It does not interrupt the turn.
func (m chatModel) confirmView() string {
	if m.ask == nil {
		return ""
	}
	width := m.width
	if width < 1 {
		width = 1
	}
	hint := hintKeyStyle.Render("enter") + hintLabelStyle.Render(" allow") +
		sepStyle.Render(" · ") +
		hintKeyStyle.Render("esc") + hintLabelStyle.Render(" deny")
	lines := []string{
		fitLine(statusBadStyle.Render("! allow this command?"), width),
		fitLine(oneLine(m.ask.command), width),
		fitLine(dimStyle.Render(oneLine(m.ask.reason)), width),
		fitLine(hint, width),
	}
	return strings.Join(lines, "\n")
}

func fitLine(s string, width int) string {
	if ansi.StringWidth(s) > width {
		return ansi.Truncate(s, width, "…")
	}
	return s
}
