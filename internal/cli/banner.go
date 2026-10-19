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
	cardFallbackWidth = 80
	cardBorderCols    = 2
	cardPadCols       = 2
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
	width := m.width
	if width > 0 {
		hint = wrapText(hint, width)
	}
	return dimStyle.Render(hint)
}

// sessionCard is the bordered session line: version, model, and directory.
func (m chatModel) sessionCard() string {
	width := m.width
	if width < 1 {
		width = cardFallbackWidth
	}
	title := "pigo " + version
	meta := sessionMeta(m.modelName, m.root)
	if width < cardMinWidth {
		if meta == "" {
			return title
		}
		return title + "\n" + meta
	}
	inner := width - cardBorderCols
	top := cardBorderStyle.Render("╭" + strings.Repeat("─", inner) + "╮")
	bot := cardBorderStyle.Render("╰" + strings.Repeat("─", inner) + "╯")
	lines := []string{top, cardRow(title, false, inner)}
	if meta != "" {
		lines = append(lines, cardRow(meta, true, inner))
	}
	lines = append(lines, bot)
	return strings.Join(lines, "\n")
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

func cardRow(text string, dim bool, inner int) string {
	room := inner - cardPadCols
	if room < 1 {
		room = 1
	}
	text = oneLine(text)
	if ansi.StringWidth(text) > room {
		text = ansi.Truncate(text, room, "…")
	}
	pad := room - ansi.StringWidth(text)
	shown := text + strings.Repeat(" ", pad)
	if dim {
		shown = dimStyle.Render(shown)
	}
	return cardBorderStyle.Render("│ ") + shown + cardBorderStyle.Render(" │")
}
