// Frame layout.
//
// The transcript fills the space above a fixed band: an optional working
// or confirm row, the slash menu, the composer, and the status bar. View
// declares the alternate screen and asks Bubble Tea to park the hardware
// cursor on the composer cell.

package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// gutterCols is the "> " / "• " prefix.
	gutterCols = 2
	// previewLines is how much of a tool result stays in the transcript.
	previewLines = 3
)

func (m chatModel) View() tea.View {
	body, x, y := m.layout()
	v := tea.NewView(body)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	placeCursor(&v, x, y, m.ready && !m.quitting && body != "")
	return v
}

// layout is the frame and the caret cell inside it. y counts from the top.
func (m *chatModel) layout() (body string, x, y int) {
	// Quit and the first paint send an empty frame. The alternate screen
	// stays on, so shutdown restores the shell instead of erasing it.
	if !m.ready || m.quitting {
		return "", 0, 0
	}
	input, x, cy := m.inputBlock()
	above := m.chromeAbove()
	bottom := joinChrome(above, input, m.footer())
	head := m.transcriptWindow(roomFor(m.height, bottom))
	y = stackHeight(head) + stackHeight(above) + cy
	body = strings.Join(append(head, bottom...), "\n")
	body, dropped := fitHeightDrop(body, m.height)
	return body, x, max(y-dropped, 0)
}

// joinChrome is the fixed band under the transcript: chrome, composer,
// a blank line, and the status bar.
func joinChrome(above []string, input, footer string) []string {
	parts := make([]string, 0, len(above)+3)
	parts = append(parts, above...)
	return append(parts, input, "", footer)
}

func roomFor(height int, parts []string) int {
	return max(height-stackHeight(parts), 0)
}

// chromeAbove is the fixed band between the transcript and the composer.
func (m *chatModel) chromeAbove() []string {
	above := []string{""} // blank line under the transcript
	if panel := m.confirmView(); panel != "" {
		// The confirm panel takes the working-row slot. Esc denies the
		// command, so the interrupt hint stays hidden.
		above = append(above, panel, "")
	} else if row := m.workingRow(); row != "" {
		above = append(above, row, "")
	}
	if menu := m.suggestView(); menu != "" {
		above = append(above, menu)
	}
	return above
}

func (m *chatModel) footer() string {
	return styleFooter(m.statusWord(), m.modelName, m.root, m.width)
}

// workingRow is the activity line above the composer: spinner, elapsed time,
// and the key that stops the turn.
func (m *chatModel) workingRow() string {
	if !m.busy {
		return ""
	}
	width := m.width
	lead := m.spin.View()
	if ansi.Strip(lead) == "" {
		lead = statusWorkStyle.Render("•")
	}
	clock := fmtElapsed(time.Since(m.started))
	line := lead + " " + statusWorkStyle.Render("Working") + " " + dimStyle.Render("("+clock+" • esc to interrupt)")
	return fitLine(line, width)
}

func (m *chatModel) statusWord() string {
	if m.busy {
		return "working"
	}
	return m.status
}

// footerHints uses the same interrupt key as the working row. Esc stops a
// running turn. Ctrl-C also stops it, and quits when the turn is idle.
const footerHints = "enter send · shift+enter newline · esc to interrupt · /exit"

func styleFooter(state, model, root string, width int) string {
	left, right := footerSides(state, model, root, width)
	if right == "" {
		return statusStyle(state).Render(left)
	}
	return statusStyle(state).Render(left) + sepStyle.Render(" · ") + hintLabelStyle.Render(right)
}

func statusStyle(state string) lipgloss.Style {
	switch state {
	case "working":
		return statusWorkStyle
	case "interrupted", "error":
		return statusBadStyle
	default:
		return statusReadyStyle
	}
}

// footerSides is the status, model, and path on the left, and the keys on
// the right. The left side shrinks first.
func footerSides(state, model, root string, width int) (string, string) {
	left := oneLine(state) + " · " + oneLine(model) + " · " + oneLine(root)
	right := footerHints
	sep := ansi.StringWidth(" · ")
	room := width - ansi.StringWidth(right) - sep
	if room < 1 {
		return ansi.Truncate(left, width, "…"), ""
	}
	return fitLine(left, room), right
}

func (m *chatModel) renderBlock(b block) string {
	switch b.kind {
	case kindUser:
		return m.paintUser(b.text)
	case kindAssistant:
		return m.paintAssistant(b.text)
	case kindRan:
		return m.paintLines(b.text, toolHeadStyle)
	case kindTool:
		return m.paintLines(b.text, dimStyle)
	case kindWorked:
		return m.workedRule(b.text)
	case kindInfo:
		return m.paintLines(b.text, dimStyle)
	case kindError:
		return m.paintLines(b.text, errStyle)
	default:
		return m.paintLines(b.text, dimStyle)
	}
}

// workedRule is the dim separator under a finished turn.
func (m *chatModel) workedRule(label string) string {
	width := m.width
	lead := "─ " + label + " "
	if ansi.StringWidth(lead) >= width {
		return dimStyle.Render(ansi.Truncate(lead, width, ""))
	}
	rule := lead + strings.Repeat("─", width-ansi.StringWidth(lead))
	return dimStyle.Render(rule)
}

func (m *chatModel) paintUser(text string) string {
	width := m.width
	lines := strings.Split(wrapText(text, textCols(width)), "\n")
	var b strings.Builder
	// Blank rows match the composer, so the sent message looks like the
	// gray block that just left the input.
	b.WriteString(paintBar("", width))
	for i, line := range lines {
		b.WriteByte('\n')
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		b.WriteString(paintBar(prefix+line, width))
	}
	b.WriteByte('\n')
	b.WriteString(paintBar("", width))
	return b.String()
}

func (m *chatModel) paintAssistant(text string) string {
	width := m.width
	lines := strings.Split(styleProse(wrapText(text, textCols(width))), "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = bulletStyle.Render("•") + " " + line
		} else {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}

// textCols is the width inside the "> " / "• " gutter.
func textCols(width int) int {
	return max(width-gutterCols, 1)
}

func (m *chatModel) paintLines(text string, style lipgloss.Style) string {
	width := m.width
	lines := strings.Split(text, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(style.Render(wrapText(line, width)))
	}
	return b.String()
}

func toolPreview(result string) string {
	result = strings.TrimRight(strings.ReplaceAll(result, "\r\n", "\n"), "\n")
	if result == "" {
		return ""
	}
	lines := strings.Split(result, "\n")
	n := min(len(lines), previewLines)
	var b strings.Builder
	for i := range n {
		if i > 0 {
			b.WriteByte('\n')
		}
		line := afterCarriage(lines[i])
		if i == 0 {
			b.WriteString("└ " + line)
			continue
		}
		b.WriteString("  " + line)
	}
	if extra := len(lines) - n; extra > 0 {
		b.WriteString("\n  + ")
		b.WriteString(strconv.Itoa(extra))
		b.WriteString(" lines")
	}
	return b.String()
}

// Progress output rewrites a line with \r. The text after the last one
// is what should stay, or it paints over the └ prefix.
func afterCarriage(line string) string {
	if cut := strings.LastIndexByte(line, '\r'); cut >= 0 {
		return line[cut+1:]
	}
	return line
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}

func stackHeight(parts []string) int {
	n := 0
	for _, part := range parts {
		n += partHeight(part)
	}
	return n
}

func partHeight(s string) int {
	if s == "" {
		return 1
	}
	return lipgloss.Height(s)
}

// fmtElapsed is the compact clock on the working row: 3s, 1m 05s, 1h 02m 03s.
func fmtElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm %02ds", s/60, s%60)
	default:
		return fmt.Sprintf("%dh %02dm %02ds", s/3600, (s%3600)/60, s%60)
	}
}

func fitHeightDrop(s string, height int) (string, int) {
	if height <= 0 || s == "" {
		return s, 0
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= height {
		return s, 0
	}
	drop := len(lines) - height
	return strings.Join(lines[drop:], "\n"), drop
}
