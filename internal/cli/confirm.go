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

func (m chatModel) answerConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.finishConfirm(true), nil
	case "esc":
		return m.finishConfirm(false), nil
	}
	switch msg.Text {
	case "y", "Y":
		return m.finishConfirm(true), nil
	case "n", "N":
		return m.finishConfirm(false), nil
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
