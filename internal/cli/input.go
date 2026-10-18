package cli

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const askPlaceholder = "Ask pigo to do anything"

// composer is the text in the input bar. The real terminal cursor is moved
// onto the caret after each frame so an IME popup opens there.
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

func (c *composer) set(s string) {
	c.runes = []rune(s)
	c.caret = len(c.runes)
}

func (c composer) text() string { return string(c.runes) }

func (m chatModel) onComposer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if msg.Alt {
			return m.newline()
		}
		return m.onEnter()
	case tea.KeyTab:
		if m.menuOpen() {
			m.completeSuggest()
		}
		return m, nil
	case tea.KeyEsc:
		m.suggestDismissed = true
		return m, nil
	case tea.KeyUp:
		if m.menuOpen() {
			m.moveSuggest(-1)
		}
		return m, nil
	case tea.KeyDown:
		if m.menuOpen() {
			m.moveSuggest(1)
		}
		return m, nil
	case tea.KeyCtrlJ:
		return m.newline()
	case tea.KeyRunes:
		m.comp.insert(string(msg.Runes))
		m.noteTyped()
		return m, nil
	case tea.KeySpace:
		m.comp.insert(" ")
		m.noteTyped()
		return m, nil
	case tea.KeyBackspace:
		m.comp.backspace()
		m.noteTyped()
		return m, nil
	case tea.KeyDelete:
		m.comp.del()
		m.noteTyped()
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
		m.noteTyped()
		return m, nil
	case tea.KeyCtrlW:
		m.comp.wordBack()
		m.noteTyped()
		return m, nil
	default:
		if msg.String() == "alt+enter" {
			return m.newline()
		}
		return m, nil
	}
}

const (
	inputPrompt     = "> "
	minInputRows    = 3
	maxContentLines = 8
)

// inputBlock draws the gray composer. x and y are the caret cell inside this
// block: y counts from the top of the block, x is the display column.
func (m chatModel) inputBlock() (view string, x, y int) {
	width := m.width
	if width < 1 {
		width = 1
	}
	gap := ansi.StringWidth(inputPrompt)
	inner := width - gap
	if m.comp.caret == len(m.comp.runes) && inner > 1 {
		// Leave a cell so a caret at the end does not wrap onto the next row.
		inner--
	}
	if inner < 1 {
		inner = 1
	}
	rows, caretLine, caretCol := wrapRunes(m.comp.runes, m.comp.caret, inner)
	if len(rows) == 0 {
		rows = [][]rune{{}}
	}
	total := m.barRows(len(rows))
	pad := total >= minInputRows
	slots := total
	if pad {
		slots = total - 2
	}
	if slots < 1 {
		slots = 1
		pad = false
	}
	start := 0
	if len(rows) > slots {
		start = caretLine - slots + 1
		if start < 0 {
			start = 0
		}
		if start+slots > len(rows) {
			start = len(rows) - slots
		}
	}
	var b strings.Builder
	y = 0
	if pad {
		b.WriteString(paintBar("", width))
		b.WriteByte('\n')
		y = 1
	}
	for i := start; i < len(rows) && i < start+slots; i++ {
		if i > start {
			b.WriteByte('\n')
		}
		prefix := strings.Repeat(" ", gap)
		if i == 0 {
			prefix = inputPrompt
		}
		if len(m.comp.runes) == 0 && i == 0 {
			b.WriteString(paintPlaceholder(width))
		} else {
			b.WriteString(paintBar(prefix+string(rows[i]), width))
		}
		if i == caretLine {
			y = 0
			if pad {
				y = 1
			}
			y += i - start
			x = caretColumn(prefix, rows[i], caretCol)
		}
	}
	if pad {
		b.WriteByte('\n')
		b.WriteString(paintBar("", width))
	}
	if x >= width {
		x = width - 1
	}
	if x < 0 {
		x = 0
	}
	return b.String(), x, y
}

// barRows is the composer height, including the blank lines above and below.
func (m chatModel) barRows(content int) int {
	want := content + 2
	if want < minInputRows {
		want = minInputRows
	}
	max := m.inputRows()
	if want > max {
		want = max
	}
	if want < 1 {
		return 1
	}
	return want
}

func (m chatModel) inputRows() int {
	room := m.height - 1
	if room < 1 {
		return 1
	}
	if m.live != "" || m.menuOpen() {
		if room > 1 {
			room--
		}
	}
	const maxRows = maxContentLines + 2
	if room > maxRows {
		return maxRows
	}
	return room
}

func caretColumn(prefix string, line []rune, col int) int {
	if col > len(line) {
		col = len(line)
	}
	return ansi.StringWidth(prefix + string(line[:col]))
}

func paintBar(line string, width int) string {
	w := ansi.StringWidth(line)
	if w > width {
		line = ansi.Truncate(line, width, "")
		w = ansi.StringWidth(line)
	}
	if w < width {
		line += strings.Repeat(" ", width-w)
	}
	return barStyle.Render(line)
}

func paintPlaceholder(width int) string {
	room := width - ansi.StringWidth(inputPrompt)
	if room < 1 {
		return paintBar(inputPrompt, width)
	}
	hint := ansi.Truncate(askPlaceholder, room, "…")
	pad := room - ansi.StringWidth(hint)
	if pad < 0 {
		pad = 0
	}
	return barStyle.Render(inputPrompt) + placeholderStyle.Render(hint+strings.Repeat(" ", pad))
}

// Keep the style values next to the composer so the input bar is one piece.
var (
	barStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252"))
	placeholderStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("243"))
)
