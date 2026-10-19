// Opening banner: the wordmark, the session card, and the hint line.

package cli

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// wordmarkMinHeight is the terminal height that still fits the wordmark
	// plus the session card without crowding the composer.
	wordmarkMinHeight = 22
	cardMinWidth      = 12
)

// mark is the pigo wordmark. Columns are p, i, g, o. Four rows, strokes
// two columns wide, with a counter or a stem on every letter.
var mark = [][4]string{
	{"████████", "██████", "████████", "████████"},
	{"██    ██", "  ██  ", "██      ", "██    ██"},
	{"████████", "  ██  ", "██  ████", "██    ██"},
	{"██      ", "██████", "████████", "████████"},
}

// wordmarkGap is the clear space between letters.
const wordmarkGap = "  "

func wordmark() string {
	colors := []lipgloss.Style{cyanStyle, coralStyle, goldStyle, cyanStyle}
	var b strings.Builder
	for i, row := range mark {
		if i > 0 {
			b.WriteByte('\n')
		}
		for j, col := range row {
			if j > 0 {
				b.WriteString(wordmarkGap)
			}
			b.WriteString(colors[j].Render(col))
		}
	}
	return b.String()
}

func (m chatModel) opening() string {
	var parts []string
	mark := m.height >= wordmarkMinHeight && m.width >= wordmarkCols()
	if mark {
		parts = append(parts, wordmark())
	}
	parts = append(parts, m.sessionCard(), m.hintLine())
	body := strings.Join(parts, "\n\n")
	if !mark {
		return body
	}
	// One blank line above the wordmark, the same gap the cells use.
	return "\n" + body
}

func wordmarkCols() int {
	if len(mark) == 0 || len(mark[0]) == 0 {
		return 0
	}
	n := 0
	for _, col := range mark[0] {
		n += ansi.StringWidth(col)
	}
	if gaps := len(mark[0]) - 1; gaps > 0 {
		n += gaps * ansi.StringWidth(wordmarkGap)
	}
	return n
}

func (m chatModel) hintLine() string {
	hint := "/skill:name loads a skill. /reset clears the chat. /exit quits."
	return dimStyle.Render(wrapText(hint, m.width))
}

// sessionCard is the bordered session line: version, model, and directory.
func (m chatModel) sessionCard() string {
	width := m.width
	title := "pigo " + version
	meta := sessionMeta(m.modelName, m.root)
	if width < cardMinWidth {
		if meta == "" {
			return title
		}
		return title + "\n" + meta
	}
	body := title
	if meta != "" {
		body += "\n" + dimStyle.Render(meta)
	}
	return cardStyle.Width(width).Render(body)
}

func sessionMeta(model, root string) string {
	model = oneLine(model)
	root = oneLine(root)
	switch {
	case model != "" && root != "":
		return model + " · " + root
	case model != "":
		return model
	default:
		return root
	}
}
