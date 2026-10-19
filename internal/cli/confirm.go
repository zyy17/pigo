// Command-guard confirm.
//
// A dangerous shell command replaces the working row until the user
// answers. Enter or y allows it. Esc or n denies it. Esc does not
// interrupt the turn; Ctrl-C still does, and denies on the way out.

package cli

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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

func (m *chatModel) answerConfirm(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.finishConfirm(true)
	case "esc":
		m.finishConfirm(false)
	default:
		switch msg.Text {
		case "y", "Y":
			m.finishConfirm(true)
		case "n", "N":
			m.finishConfirm(false)
		}
	}
	return nil
}

func (m *chatModel) finishConfirm(allow bool) {
	if m.ask == nil {
		return
	}
	select {
	case m.ask.reply <- allow:
	default:
	}
	m.ask = nil
}

// confirmView replaces the working row while a dangerous command is waiting.
// Esc denies that command. It does not interrupt the turn.
func (m *chatModel) confirmView() string {
	if m.ask == nil {
		return ""
	}
	width := m.width
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
	return ansi.Truncate(s, width, "…")
}
