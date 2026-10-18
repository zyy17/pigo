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
	// An empty view lets Bubble Tea erase the input and footer on quit.
	// Finished lines were already printed into the scrollback.
	if !m.ready || m.quitting {
		return ""
	}
	m.fitInput()
	parts := make([]string, 0, 3)
	if live := m.liveView(); live != "" {
		parts = append(parts, live)
	}
	parts = append(parts, m.inputBox(), m.footer())
	return fitHeight(lipgloss.JoinVertical(lipgloss.Left, parts...), m.height)
}

func (m chatModel) liveView() string {
	if m.live == "" {
		return ""
	}
	text := m.live
	if m.width > 0 {
		text = wrapPlain(text, m.width)
	}
	lines := strings.Split(renderAssistant(text), "\n")
	limit := m.liveLimit()
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n")
}

func (m chatModel) liveLimit() int {
	rest := m.height - lipgloss.Height(m.inputBox()) - lipgloss.Height(m.footer())
	if rest < 1 {
		return 1
	}
	return rest
}

func (m chatModel) inputBox() string {
	return inputStyle.Width(m.boxWidth()).Render(m.input.View())
}

func (m chatModel) footer() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	state := "ready"
	lead := ""
	if m.busy {
		state = "working"
		lead = m.spin.View() + " "
	}
	leadW := lipgloss.Width(lead)
	inner := width - leadW
	if inner < 8 {
		inner = width
		lead = ""
	}
	root := flattenLine(m.root, 32)
	status := wrapPlain(state+" · "+m.modelName+" · "+root, inner)
	keys := wrapPlain("enter send · alt+enter newline · ctrl-c abort · /exit", width)
	one := state + " · " + m.modelName + " · " + root + " · " + "enter send · alt+enter newline · ctrl-c abort · /exit"
	if lipgloss.Width(one)+leadW <= width {
		line := dimStyle.Render(one)
		if lead == "" {
			return line
		}
		return lead + line
	}
	lines := strings.Split(dimStyle.Render(status+"\n"+keys), "\n")
	if lead != "" && len(lines) > 0 {
		lines[0] = lead + lines[0]
	}
	return strings.Join(lines, "\n")
}

func fitHeight(s string, height int) string {
	if height <= 0 || s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= height {
		return s
	}
	return strings.Join(lines[len(lines)-height:], "\n")
}

func wrapPlain(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, wrapLine(line, width)...)
	}
	return strings.Join(out, "\n")
}

func wrapLine(line string, width int) []string {
	r := []rune(line)
	if len(r) == 0 {
		return []string{""}
	}
	var out []string
	for len(r) > width {
		out = append(out, string(r[:width]))
		r = r[width:]
	}
	out = append(out, string(r))
	return out
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
