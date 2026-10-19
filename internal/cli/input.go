// Composer: the text under the prompt, and the gray bar it is drawn in.
//
// The caret cell is returned with the bar so the view can place the
// hardware cursor there. Wide characters count as two columns.

package cli

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

const (
	askPlaceholder = "Ask pigo to do anything"
	inputPrompt    = "> "

	minInputRows    = 3
	maxContentLines = 8
	// composerPadRows is the blank line above the prompt and the one below it.
	composerPadRows = 2
	// footerBandRows is the status bar plus the blank line above it.
	footerBandRows = 2
	// endCaretSlack keeps a caret at the end of a full line from wrapping.
	endCaretSlack = 1
)

// composer is the text in the input bar. caret is a rune index.
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

// inputBlock draws the gray composer. x and y are the caret cell inside this
// block: y counts from the top of the block, x is the display column.
func (m *chatModel) inputBlock() (view string, x, y int) {
	width := m.width
	gap := ansi.StringWidth(inputPrompt)
	inner := max(width-gap, 1)
	if m.comp.caret == len(m.comp.runes) && inner > endCaretSlack {
		inner -= endCaretSlack
	}
	inner = max(inner, 1)
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
	start := windowStart(len(rows), slots, caretLine)
	var b strings.Builder
	padTop := 0
	if pad {
		b.WriteString(paintBar("", width))
		b.WriteByte('\n')
		padTop = 1
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
	}
	if pad {
		b.WriteByte('\n')
		b.WriteString(paintBar("", width))
	}
	prefix := strings.Repeat(" ", gap)
	if caretLine == 0 {
		prefix = inputPrompt
	}
	line := rows[min(caretLine, len(rows)-1)]
	x = min(max(caretColumn(prefix, line, caretCol), 0), max(width-1, 0))
	y = max(padTop+caretLine-start, 0)
	return b.String(), x, y
}

// barRows is the composer height, including the blank lines above and below.
func (m *chatModel) barRows(content int) int {
	want := max(content+composerPadRows, minInputRows)
	return min(want, m.inputRows())
}

// inputRows is the tallest the composer may be. The chrome above it is
// measured, so a slash menu of up to suggestWindow lines is already counted.
func (m *chatModel) inputRows() int {
	room := m.height - stackHeight(m.chromeAbove()) - footerBandRows
	return min(max(room, 1), maxContentLines+composerPadRows)
}

// windowStart is the first wrapped row still visible, keeping the caret row on screen.
func windowStart(n, slots, caret int) int {
	if slots < 1 || n <= slots {
		return 0
	}
	start := max(caret-slots+1, 0)
	if start+slots > n {
		return n - slots
	}
	return start
}

func caretColumn(prefix string, line []rune, col int) int {
	col = min(col, len(line))
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
	pad := max(room-ansi.StringWidth(hint), 0)
	return barStyle.Render(inputPrompt) + placeholderStyle.Render(hint+strings.Repeat(" ", pad))
}
