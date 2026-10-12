package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	userStyle  = lipgloss.NewStyle().Bold(true)
	toolStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func (m chatModel) View() string {
	if !m.ready {
		return "starting…"
	}
	header := titleStyle.Render("pigo") + "  " + dimStyle.Render(m.session.Root()+"  "+m.session.Model())
	status := "ready"
	if m.busy {
		status = "working"
	}
	help := dimStyle.Render(status + "   enter send   pgup/pgdown scroll   ctrl+c cancel   /exit quit")
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.view.View(),
		help,
		m.input.View(),
	)
}

func renderTranscript(lines []chatLine) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprint(&b, styleLine(line))
	}
	if len(lines) > 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

func styleLine(line chatLine) string {
	switch line.kind {
	case kindUser:
		return userStyle.Render("you> " + line.text)
	case kindTool:
		return toolStyle.Render(line.text)
	case kindError:
		return errStyle.Render(line.text)
	case kindInfo:
		return dimStyle.Render(line.text)
	default:
		return line.text
	}
}
