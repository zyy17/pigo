// Display-width wrapping.
//
// Wide characters (CJK) count as two columns. Spaces break first; a token
// wider than the line is hard-wrapped. The composer uses wrapRunes so the
// caret stays on the same rune after a wrap.
package cli

import "github.com/charmbracelet/x/ansi"

func wrapText(s string, width int) string {
	if width < 1 || s == "" {
		return s
	}
	s = ansi.Wordwrap(s, width, "")
	return ansi.Hardwrap(s, width, true)
}

func runeWidth(r rune) int {
	if r == '\n' {
		return 0
	}
	w := ansi.StringWidth(string(r))
	if w < 1 {
		return 1
	}
	return w
}

// wrapRunes splits text on newlines, then on spaces, then on display width.
// caret is a rune index. line and col say which wrapped row holds it.
func wrapRunes(rs []rune, caret, width int) (lines [][]rune, line, col int) {
	if width < 1 {
		width = 1
	}
	if caret > len(rs) {
		caret = len(rs)
	}
	var (
		cur      []rune
		w        int
		space    = -1
		consumed []int
	)
	push := func(extra int) {
		lines = append(lines, cur)
		consumed = append(consumed, len(cur)+extra)
		cur = nil
		w = 0
		space = -1
	}
	// breakWord moves the tail after the last space onto the next row.
	breakWord := func() {
		if space > 0 && space < len(cur)-1 {
			rest := append([]rune(nil), cur[space+1:]...)
			cur = cur[:space+1]
			push(0)
			cur = rest
			for i, r := range cur {
				w += runeWidth(r)
				if r == ' ' {
					space = i
				}
			}
			return
		}
		push(0)
	}
	for _, r := range rs {
		if r == '\n' {
			push(1)
			continue
		}
		rw := runeWidth(r)
		if len(cur) > 0 && w+rw > width {
			breakWord()
			if len(cur) > 0 && w+rw > width {
				push(0)
			}
		}
		cur = append(cur, r)
		w += rw
		if r == ' ' {
			space = len(cur) - 1
		}
	}
	lines = append(lines, cur)
	consumed = append(consumed, len(cur))

	left := caret
	for i := range lines {
		if left <= len(lines[i]) {
			return lines, i, left
		}
		// A newline sits between rows and counts as one source rune.
		gap := consumed[i] - len(lines[i])
		left -= len(lines[i]) + gap
	}
	last := len(lines) - 1
	return lines, last, len(lines[last])
}
