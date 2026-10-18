package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// mark is the pigo wordmark. Columns are p, i, g, o.
var mark = [][4]string{
	{"██████", "██", "██████", "██████"},
	{"██  ██", "██", "██    ", "██  ██"},
	{"██████", "██", "██ ███", "██  ██"},
	{"██    ", "██", "██  ██", "██  ██"},
	{"██    ", "██", "██████", "██████"},
}

func wordmark() string {
	colors := []lipgloss.Style{cyanStyle, coralStyle, goldStyle, cyanStyle}
	var b strings.Builder
	for i, row := range mark {
		if i > 0 {
			b.WriteByte('\n')
		}
		for j, col := range row {
			if j > 0 {
				b.WriteString("  ")
			}
			b.WriteString(colors[j].Render(col))
		}
	}
	return b.String()
}

func (m chatModel) header() string {
	line := dimStyle.Render("pigo " + version)
	if m.height < 22 || m.width < 40 {
		return line
	}
	return wordmark() + "\n" + line
}

func (m chatModel) opening() string {
	hint := "/skill:name loads a skill. /reset clears the chat. /exit quits."
	if m.width > 0 {
		hint = wrapPlain(hint, m.width)
	}
	return m.header() + "\n" + dimStyle.Render(hint)
}
