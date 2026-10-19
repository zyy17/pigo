// Frame layout.
//
// The transcript fills the space above a fixed band: an optional working
// or confirm row, the slash menu, the composer, and the status bar. View
// declares the alternate screen and asks Bubble Tea to park the hardware
// cursor on the composer cell.
package cli

import (
	"encoding/json"
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

type footerKind int

const (
	footStatus footerKind = iota
	footSep
	footModel
	footPath
	footKey
	footLabel
)

type footerPiece struct {
	kind footerKind
	text string
}

func (m chatModel) View() tea.View {
	body, x, y := m.layout()
	v := tea.NewView(body)
	// Keep the alternate screen on the quit frame too. Shutdown then
	// leaves that screen. An empty main-screen frame would erase the
	// shell lines below the cursor instead of restoring them.
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	placeCursor(&v, x, y, m.ready && !m.quitting && body != "")
	return v
}

// layout is the frame and the caret cell inside it. y counts from the top.
func (m chatModel) layout() (body string, x, y int) {
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
	y -= dropped
	if y < 0 {
		y = 0
	}
	return body, x, y
}

// joinChrome is the fixed band under the transcript: chrome, composer,
// a blank line, and the status bar.
func joinChrome(above []string, input, footer string) []string {
	parts := make([]string, 0, len(above)+3)
	parts = append(parts, above...)
	return append(parts, input, "", footer)
}

func roomFor(height int, parts []string) int {
	room := height - stackHeight(parts)
	if room < 0 {
		return 0
	}
	return room
}

// chromeAbove is the fixed band between the transcript and the composer.
func (m chatModel) chromeAbove() []string {
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

func (m chatModel) footer() string {
	width := m.width
	if width < 1 {
		width = 1
	}
	return styleFooter(m.statusWord(), m.modelName, m.root, width)
}

// workingRow is the activity line above the composer: spinner, elapsed time,
// and the key that stops the turn.
func (m chatModel) workingRow() string {
	if !m.busy {
		return ""
	}
	width := m.width
	if width < 1 {
		width = 1
	}
	lead := m.spin.View()
	if ansi.Strip(lead) == "" {
		lead = statusWorkStyle.Render("•")
	}
	clock := fmtElapsed(time.Since(m.started))
	line := lead + " " + statusWorkStyle.Render("Working") + " " + dimStyle.Render("("+clock+" • esc to interrupt)")
	if ansi.StringWidth(line) > width {
		return ansi.Truncate(line, width, "…")
	}
	return line
}

func (m chatModel) statusWord() string {
	if m.busy {
		return "working"
	}
	if m.status == "" {
		return "ready"
	}
	return m.status
}

// composeFooter is the plain status line. The path shrinks first, then hints
// drop from the end, and whatever remains is cut with an ellipsis.
func composeFooter(state, model, root string, width int) string {
	return piecesPlain(footerPieces(state, model, root, width))
}

func styleFooter(state, model, root string, width int) string {
	var b strings.Builder
	for _, piece := range footerPieces(state, model, root, width) {
		b.WriteString(piece.style(state).Render(piece.text))
	}
	return b.String()
}

func piecesPlain(parts []footerPiece) string {
	var b strings.Builder
	for _, piece := range parts {
		b.WriteString(piece.text)
	}
	return b.String()
}

func (p footerPiece) style(state string) lipgloss.Style {
	switch p.kind {
	case footStatus:
		return statusStyle(state)
	case footModel:
		return modelStyle
	case footPath:
		return pathStyle
	case footKey:
		return hintKeyStyle
	case footLabel:
		return hintLabelStyle
	default:
		return sepStyle
	}
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

type footerHint struct {
	key   string
	label string
}

func footerPieces(state, model, root string, width int) []footerPiece {
	if width < 1 {
		return nil
	}
	model = oneLine(model)
	root = oneLine(root)
	hints := []footerHint{
		{key: "enter", label: " send"},
		{key: "shift+enter", label: " newline"},
		{key: "ctrl-c", label: " abort"},
		{key: "/", label: "exit"},
	}
	head := []footerPiece{
		{footStatus, state},
		{footSep, " · "},
		{footModel, model},
		{footSep, " · "},
	}
	for n := len(hints); n >= 0; n-- {
		var tail []footerPiece
		for _, hint := range hints[:n] {
			tail = append(tail, footerPiece{footSep, " · "}, footerPiece{footKey, hint.key}, footerPiece{footLabel, hint.label})
		}
		room := width - piecesWidth(head) - piecesWidth(tail)
		if room < 1 {
			continue
		}
		shown := root
		if ansi.StringWidth(shown) > room {
			shown = ansi.Truncate(root, room, "…")
		}
		if shown == "" {
			continue
		}
		parts := append(append([]footerPiece{}, head...), footerPiece{footPath, shown})
		parts = append(parts, tail...)
		if piecesWidth(parts) <= width {
			return parts
		}
	}
	plain := ansi.Truncate(state+" · "+model+" · "+root, width, "…")
	return []footerPiece{{footStatus, plain}}
}

func piecesWidth(parts []footerPiece) int {
	return ansi.StringWidth(piecesPlain(parts))
}

func (m chatModel) renderBlock(b block) string {
	switch b.kind {
	case kindUser:
		return m.paintUser(b.text)
	case kindAssistant:
		return m.paintAssistant(b.text)
	case kindTool:
		return m.paintTool(b.text)
	case kindInfo:
		if strings.HasPrefix(b.text, "Worked for ") {
			return m.workedRule(b.text)
		}
		return m.paintLines(b.text, dimStyle)
	case kindError:
		return m.paintLines(b.text, errStyle)
	default:
		return m.paintLines(b.text, dimStyle)
	}
}

// workedRule is the dim separator under a finished turn.
func (m chatModel) workedRule(label string) string {
	width := m.width
	if width < 1 {
		width = ansi.StringWidth(label) + 2
	}
	lead := "─ " + label + " "
	if ansi.StringWidth(lead) >= width {
		return dimStyle.Render(ansi.Truncate(lead, width, ""))
	}
	rule := lead + strings.Repeat("─", width-ansi.StringWidth(lead))
	return dimStyle.Render(rule)
}

func (m chatModel) paintTool(text string) string {
	style := dimStyle
	if strings.HasPrefix(text, "• Ran ") {
		style = toolHeadStyle
	}
	return m.paintLines(text, style)
}

func (m chatModel) paintUser(text string) string {
	width := paneCols(m.width)
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

func (m chatModel) paintAssistant(text string) string {
	width := paneCols(m.width)
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

func paneCols(width int) int {
	if width < 1 {
		return 1
	}
	return width
}

// textCols is the width inside the "> " / "• " gutter.
func textCols(width int) int {
	if width < gutterCols+1 {
		return 1
	}
	return width - gutterCols
}

func (m chatModel) paintLines(text string, style lipgloss.Style) string {
	width := m.width
	if width < 1 {
		return style.Render(text)
	}
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

func ranTitle(name string, args []byte) string {
	var fields map[string]any
	_ = json.Unmarshal(args, &fields)
	if cmd, _ := fields["command"].(string); cmd != "" {
		return "• Ran " + oneLine(cmd)
	}
	if path, _ := fields["path"].(string); path != "" {
		return "• Ran " + name + " " + path
	}
	return "• Ran " + name
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
		if i == 0 {
			b.WriteString("└ " + lines[i])
			continue
		}
		b.WriteString("  " + lines[i])
	}
	if extra := len(lines) - n; extra > 0 {
		b.WriteString("\n  + ")
		b.WriteString(strconv.Itoa(extra))
		b.WriteString(" lines")
	}
	return b.String()
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
	if s < minuteSeconds {
		return strconv.Itoa(s) + "s"
	}
	if s < hourSeconds {
		return strconv.Itoa(s/minuteSeconds) + "m " + twoDigits(s%minuteSeconds) + "s"
	}
	return strconv.Itoa(s/hourSeconds) + "h " + twoDigits((s%hourSeconds)/minuteSeconds) + "m " + twoDigits(s%minuteSeconds) + "s"
}

const (
	minuteSeconds = 60
	hourSeconds   = 3600
)

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
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
