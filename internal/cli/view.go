package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	cyanStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	coralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	goldStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	userStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082")).Bold(true)
	toolStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	headStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58")).Bold(true)
	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#00ADD8")).
			Padding(0, 1)
)

func (m chatModel) View() string {
	if !m.ready {
		return "starting…"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.header(),
		m.view.View(),
		m.inputBox(),
		m.footer(),
	)
}

func (m chatModel) inputBox() string {
	return inputStyle.Width(m.width).Render(m.input.View())
}

func (m chatModel) footer() string {
	state := "ready"
	mark := ""
	if m.busy {
		state = "working"
		mark = m.spin.View() + " "
	}
	hint := state + "  ·  " + m.modelName + "  ·  " + flattenLine(m.root, 32) +
		"  ·  enter send  ·  alt+enter newline  ·  ctrl-c abort  ·  /exit"
	return mark + dimStyle.Render(hint)
}

func renderTranscript(lines []chatLine) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
			if line.kind == kindUser {
				b.WriteByte('\n')
			}
		}
		b.WriteString(styleLine(line))
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
		return renderAssistant(line.text)
	}
}

func renderAssistant(text string) string {
	lines := strings.Split(text, "\n")
	var b strings.Builder
	code := false
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			code = !code
			b.WriteString(dimStyle.Render(line))
			continue
		}
		if code {
			b.WriteString(cyanStyle.Render(line))
			continue
		}
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "#"); ok {
			b.WriteString(headStyle.Render(strings.TrimSpace(strings.TrimLeft(rest, "#"))))
			continue
		}
		b.WriteString(renderInline(line))
	}
	return b.String()
}

func renderInline(line string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(line, '`')
		if i < 0 {
			b.WriteString(renderBold(line))
			return b.String()
		}
		j := strings.IndexByte(line[i+1:], '`')
		if j < 0 {
			b.WriteString(renderBold(line))
			return b.String()
		}
		j += i + 1
		b.WriteString(renderBold(line[:i]))
		b.WriteString(cyanStyle.Render(line[i+1 : j]))
		line = line[j+1:]
	}
}

func renderBold(line string) string {
	var b strings.Builder
	for {
		i := strings.Index(line, "**")
		if i < 0 {
			b.WriteString(line)
			return b.String()
		}
		j := strings.Index(line[i+2:], "**")
		if j < 0 {
			b.WriteString(line)
			return b.String()
		}
		j += i + 2
		b.WriteString(line[:i])
		b.WriteString(headStyle.Render(line[i+2 : j]))
		line = line[j+2:]
	}
}
