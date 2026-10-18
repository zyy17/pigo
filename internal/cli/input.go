package cli

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const askPlaceholder = "Ask pigo to do anything"

// composer is the input line. It draws the caret itself. A textarea caret is
// a cell in its own buffer, and once that buffer is wider than the screen the
// terminal wraps the caret onto the footer.
type composer struct {
	runes []rune
	caret int
}

func (c *composer) insert(s string) {
	r := []rune(s)
	if len(r) == 0 {
		return
	}
	c.runes = append(c.runes[:c.caret], append(r, c.runes[c.caret:]...)...)
	c.caret += len(r)
}

func (c *composer) backspace() {
	if c.caret == 0 {
		return
	}
	c.runes = append(c.runes[:c.caret-1], c.runes[c.caret:]...)
	c.caret--
}

func (c *composer) del() {
	if c.caret >= len(c.runes) {
		return
	}
	c.runes = append(c.runes[:c.caret], c.runes[c.caret+1:]...)
}

func (c *composer) left() {
	if c.caret > 0 {
		c.caret--
	}
}

func (c *composer) right() {
	if c.caret < len(c.runes) {
		c.caret++
	}
}

func (c *composer) home() { c.caret = 0 }

func (c *composer) end() { c.caret = len(c.runes) }

func (c *composer) clear() {
	c.runes = nil
	c.caret = 0
}

func (c *composer) wordBack() {
	for c.caret > 0 && unicode.IsSpace(c.runes[c.caret-1]) {
		c.backspace()
	}
	for c.caret > 0 && !unicode.IsSpace(c.runes[c.caret-1]) {
		c.backspace()
	}
}

func (c composer) text() string { return string(c.runes) }

func (m chatModel) onComposer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if msg.Alt {
			m.comp.insert("\n")
			return m, nil
		}
		return m.submit(strings.TrimSpace(m.comp.text()))
	case tea.KeyCtrlJ:
		m.comp.insert("\n")
		return m, nil
	case tea.KeyRunes:
		m.comp.insert(string(msg.Runes))
		return m, nil
	case tea.KeySpace:
		m.comp.insert(" ")
		return m, nil
	case tea.KeyBackspace:
		m.comp.backspace()
		return m, nil
	case tea.KeyDelete:
		m.comp.del()
		return m, nil
	case tea.KeyLeft, tea.KeyCtrlB:
		m.comp.left()
		return m, nil
	case tea.KeyRight, tea.KeyCtrlF:
		m.comp.right()
		return m, nil
	case tea.KeyHome, tea.KeyCtrlA:
		m.comp.home()
		return m, nil
	case tea.KeyEnd, tea.KeyCtrlE:
		m.comp.end()
		return m, nil
	case tea.KeyCtrlU:
		m.comp.clear()
		return m, nil
	case tea.KeyCtrlW:
		m.comp.wordBack()
		return m, nil
	default:
		if msg.String() == "alt+enter" {
			m.comp.insert("\n")
			return m, nil
		}
		return m, nil
	}
}

func (m chatModel) inputView() string {
	width := m.width
	if width < 1 {
		width = 1
	}
	const prompt = "> "
	gap := ansi.StringWidth(prompt)
	inner := width - gap
	if inner < 2 {
		inner = 2
	}
	// A caret at the end is an extra column, so the text wraps one sooner.
	contentWidth := inner
	if m.comp.caret == len(m.comp.runes) {
		contentWidth = inner - 1
	}
	if contentWidth < 1 {
		contentWidth = 1
	}
	rows, caretLine, caretCol := wrapRunes(m.comp.runes, m.comp.caret, contentWidth)
	maxLines := m.height - 2
	if maxLines < 1 {
		maxLines = 1
	}
	start := 0
	if len(rows) > maxLines {
		start = caretLine - maxLines + 1
		if start < 0 {
			start = 0
		}
		if start+maxLines > len(rows) {
			start = len(rows) - maxLines
		}
	}
	var b strings.Builder
	for i := start; i < len(rows) && i < start+maxLines; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		prefix := strings.Repeat(" ", gap)
		if i == 0 {
			prefix = prompt
		}
		switch {
		case len(m.comp.runes) == 0 && i == 0:
			hint := ansi.Truncate(askPlaceholder, width-2, "…")
			b.WriteString(barLine(width, ">", " ", hint, true))
		case i == caretLine:
			b.WriteString(barCaretLine(width, prefix, rows[i], caretCol))
		default:
			b.WriteString(barLine(width, prefix+string(rows[i]), "", "", false))
		}
	}
	return b.String()
}

// barLine paints one full-width input row. The caret is its own cell so a
// later style pass cannot drop it or push it onto the footer.
func barLine(width int, before, caret, after string, placeholder bool) string {
	used := ansi.StringWidth(before) + ansi.StringWidth(caret) + ansi.StringWidth(after)
	pad := width - used
	if pad < 0 {
		pad = 0
	}
	var b strings.Builder
	if before != "" {
		b.WriteString(barStyle.Render(before))
	}
	if caret != "" {
		b.WriteString(caretStyle.Render(caret))
	}
	rest := after + strings.Repeat(" ", pad)
	if rest != "" {
		style := barStyle
		if placeholder {
			style = placeholderStyle
		}
		b.WriteString(style.Render(rest))
	}
	return b.String()
}

func barCaretLine(width int, prefix string, line []rune, col int) string {
	if col >= len(line) {
		return barLine(width, prefix+string(line), " ", "", false)
	}
	return barLine(width, prefix+string(line[:col]), string(line[col]), string(line[col+1:]), false)
}

// Keep the style values next to the composer so the input bar is one piece.
var (
	barStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))
	placeholderStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("243"))
	caretStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Reverse(true)
)
