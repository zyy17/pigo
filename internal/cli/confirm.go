// Confirm panel.
//
// A dangerous shell command, or a file diff, replaces the working row
// until the user answers. Enter or y allows it. Esc or n denies it.
// Esc does not interrupt the turn; Ctrl-C still does, and denies on the way out.

package cli

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zyy17/pigo/internal/coding"
)

// maxConfirmBody is how many diff lines stay on screen. The rest is a count.
const maxConfirmBody = 8

// confirmReq is one question waiting for the user.
type confirmReq struct {
	title  string
	body   string
	detail string
	reply  chan bool
}

func confirmRequest(c coding.Confirm, reply chan bool) *confirmReq {
	return &confirmReq{title: c.Title, body: c.Body, detail: c.Detail, reply: reply}
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

// confirmView replaces the working row while a command or a file diff is waiting.
// Esc denies that call. It does not interrupt the turn.
func (m *chatModel) confirmView() string {
	if m.ask == nil {
		return ""
	}
	width := m.width
	hint := hintKeyStyle.Render("enter") + hintLabelStyle.Render(" allow") +
		sepStyle.Render(" · ") +
		hintKeyStyle.Render("esc") + hintLabelStyle.Render(" deny")
	lines := []string{fitLine(statusBadStyle.Render(m.ask.title), width)}
	body, more := clipConfirm(m.ask.body, maxConfirmBody)
	for _, line := range body {
		lines = append(lines, fitLine(styleConfirmLine(line), width))
	}
	if more > 0 {
		lines = append(lines, fitLine(dimStyle.Render("… "+strconv.Itoa(more)+" more lines"), width))
	}
	if detail := oneLine(m.ask.detail); detail != "" {
		lines = append(lines, fitLine(dimStyle.Render(detail), width))
	}
	lines = append(lines, fitLine(hint, width))
	return strings.Join(lines, "\n")
}

func clipConfirm(body string, limit int) ([]string, int) {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return nil, 0
	}
	lines := strings.Split(body, "\n")
	if len(lines) <= limit {
		return lines, 0
	}
	return lines[:limit], len(lines) - limit
}

func styleConfirmLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"),
		strings.HasPrefix(line, "@@"), strings.HasPrefix(line, `\ `):
		return dimStyle.Render(line)
	case strings.HasPrefix(line, "+"):
		return diffAddStyle.Render(line)
	case strings.HasPrefix(line, "-"):
		return diffDelStyle.Render(line)
	default:
		return line
	}
}

func fitLine(s string, width int) string {
	return ansi.Truncate(s, width, "…")
}
