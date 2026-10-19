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
	// chromeGapRows is the blank line between the transcript and this band.
	chromeGapRows = 1
	// confirmBandRows is the four-line confirm panel plus the blank under it.
	confirmBandRows = 5
	// workingBandRows is the working row plus the blank under it.
	workingBandRows = 2
	menuRows        = 1
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
func (m chatModel) inputBlock() (view string, x, y int) {
	width := m.width
	if width < 1 {
		width = 1
	}
	gap := ansi.StringWidth(inputPrompt)
	inner := width - gap
	if m.comp.caret == len(m.comp.runes) && inner > endCaretSlack {
		inner -= endCaretSlack
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
	start := windowStart(len(rows), slots, caretLine)
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
	want := content + composerPadRows
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
	room := m.height - footerBandRows
	if room < 1 {
		return 1
	}
	room = reserveRows(room, chromeGapRows)
	switch {
	case m.ask != nil:
		room = reserveRows(room, confirmBandRows)
	case m.busy:
		room = reserveRows(room, workingBandRows)
	}
	if m.menuOpen() {
		room = reserveRows(room, menuRows)
	}
	if maxRows := maxContentLines + composerPadRows; room > maxRows {
		return maxRows
	}
	return room
}

// reserveRows gives back rows to the chrome above the composer, and
// always leaves the composer at least one row.
func reserveRows(room, n int) int {
	for range n {
		if room <= 1 {
			return room
		}
		room--
	}
	return room
}

// windowStart is the first wrapped row still visible, keeping the caret row on screen.
func windowStart(n, slots, caret int) int {
	if slots < 1 || n <= slots {
		return 0
	}
	start := caret - slots + 1
	if start < 0 {
		start = 0
	}
	if start+slots > n {
		return n - slots
	}
	return start
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
