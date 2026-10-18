package cli

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	cyanStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8"))
	coralStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	goldStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58"))
	dimStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	headStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58")).Bold(true)
	statusReadyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5FBF6A"))
	statusWorkStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F1BE58"))
	statusBadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F09082"))
	modelStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#00ADD8")).Bold(true)
	pathStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	hintKeyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintLabelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sepStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
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

type blockKind int

const (
	kindUser blockKind = iota
	kindAssistant
	kindTool
	kindInfo
	kindError
)

type block struct {
	kind blockKind
	text string
}

func (m chatModel) View() string {
	body, x, y := m.layout()
	if m.cur != nil {
		show := m.ready && !m.quitting && body != ""
		up := 0
		if show {
			n := strings.Count(body, "\n")
			up = n - y
			if up < 0 {
				up = 0
			}
		}
		m.cur.set(x, up, show)
	}
	return body
}

// layout is the frame and the caret cell inside it. y counts from the top.
func (m chatModel) layout() (body string, x, y int) {
	// An empty view lets Bubble Tea erase the composer on quit.
	// Finished lines were already printed into the scrollback.
	if !m.ready || m.quitting {
		return "", 0, 0
	}
	input, cx, cy := m.inputBlock()
	menu := m.suggestView()
	reserved := lipgloss.Height(input) + 1
	if menu != "" {
		reserved += lipgloss.Height(menu)
	}
	var parts []string
	if live := m.liveTail(reserved); live != "" {
		parts = append(parts, live)
	}
	if menu != "" {
		parts = append(parts, menu)
	}
	before := 0
	for _, part := range parts {
		before += strings.Count(part, "\n") + 1
	}
	parts = append(parts, input, m.footer())
	body = strings.Join(parts, "\n")
	y = before + cy
	body, dropped := fitHeightDrop(body, m.height)
	y -= dropped
	if y < 0 {
		y = 0
	}
	return body, cx, y
}

func (m chatModel) liveTail(reserved int) string {
	if m.live == "" {
		return ""
	}
	text := m.paintAssistant(m.live)
	lines := strings.Split(text, "\n")
	limit := m.height - reserved
	if limit < 1 {
		limit = 1
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n")
}

func (m chatModel) footer() string {
	width := m.width
	if width < 1 {
		width = 1
	}
	state := m.statusWord()
	lead := ""
	if m.busy {
		lead = m.spin.View() + " "
	}
	budget := width - ansi.StringWidth(lead)
	if budget < 1 {
		budget = 1
	}
	return lead + styleFooter(state, m.modelName, m.root, budget)
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
		{key: "ctrl-c", label: " abort"},
		{key: "/", label: "exit"},
		{key: "alt+enter", label: " newline"},
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
	case kindError:
		return m.paintLines(b.text, errStyle)
	default:
		return m.paintLines(b.text, dimStyle)
	}
}

func (m chatModel) paintUser(text string) string {
	width := m.width
	if width < 1 {
		width = 1
	}
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	lines := strings.Split(wrapText(text, inner), "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		b.WriteString(prefix + line)
	}
	return barStyle.Width(width).Render(b.String())
}

func (m chatModel) paintAssistant(text string) string {
	width := m.width
	if width < 1 {
		width = 1
	}
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	lines := strings.Split(renderAssistant(wrapText(text, inner)), "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = "• " + line
		} else {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
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

func (m chatModel) transcript() string {
	parts := make([]string, 0, len(m.blocks)+1)
	if m.ready {
		parts = append(parts, m.opening())
	}
	for _, b := range m.blocks {
		parts = append(parts, m.renderBlock(b))
	}
	return strings.Join(parts, "\n")
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
	const show = 3
	n := min(len(lines), show)
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

func renderAssistant(text string) string {
	lines := strings.Split(text, "\n")
	var b strings.Builder
	code := false
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			code = !code
			b.WriteString(dimStyle.Render(line))
			continue
		}
		if code {
			b.WriteString(cyanStyle.Render(line))
			continue
		}
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "#"); ok {
			b.WriteString(headStyle.Render(strings.TrimSpace(strings.TrimLeft(rest, "#"))))
			continue
		}
		b.WriteString(renderInline(line))
	}
	return b.String()
}

func renderInline(line string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(line, '`')
		if i < 0 {
			b.WriteString(renderBold(line))
			return b.String()
		}
		j := strings.IndexByte(line[i+1:], '`')
		if j < 0 {
			b.WriteString(renderBold(line))
			return b.String()
		}
		j += i + 1
		b.WriteString(renderBold(line[:i]))
		b.WriteString(cyanStyle.Render(line[i+1 : j]))
		line = line[j+1:]
	}
}

func renderBold(line string) string {
	var b strings.Builder
	for {
		i := strings.Index(line, "**")
		if i < 0 {
			b.WriteString(line)
			return b.String()
		}
		j := strings.Index(line[i+2:], "**")
		if j < 0 {
			b.WriteString(line)
			return b.String()
		}
		j += i + 2
		b.WriteString(line[:i])
		b.WriteString(headStyle.Render(line[i+2 : j]))
		line = line[j+2:]
	}
}
