package cli

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestCaretColumnForCJKAndWrap(t *testing.T) {
	m := newChat(nil, nil)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(80, 24)

	body, x, y := m.layout()
	lines := strings.Split(body, "\n")
	if lipglossHeight(body) < 4 {
		t.Fatalf("bar should be at least 3 rows plus the footer, got %d", len(lines))
	}
	plain := ansi.Strip(lines[y])
	if !strings.Contains(plain, askPlaceholder) || !strings.HasPrefix(strings.TrimRight(plain, " "), "> ") {
		t.Fatalf("placeholder line %d: %q", y, plain)
	}
	if x != 2 {
		t.Fatalf("empty caret column %d", x)
	}
	if strings.TrimSpace(ansi.Strip(lines[y-1])) != "" || strings.TrimSpace(ansi.Strip(lines[y+1])) != "" {
		t.Fatal("missing the blank lines above and below the prompt")
	}

	m.comp.insert("我们额")
	body, x, y = m.layout()
	lines = strings.Split(body, "\n")
	if x != 2+ansi.StringWidth("我们额") {
		t.Fatalf("cjk caret column %d", x)
	}
	if !strings.Contains(ansi.Strip(lines[y]), "我们额") {
		t.Fatalf("cjk not on caret row %d", y)
	}
	up := strings.Count(body, "\n") - y
	if up != 3 {
		t.Fatalf("caret should sit on the prompt, with the pad, a blank line, and the footer below, up=%d y=%d", up, y)
	}

	m.comp.set(strings.Repeat("我们", 40) + " then some english")
	_, x, y = m.layout()
	lines = strings.Split(m.View(), "\n")
	row := strings.TrimRight(ansi.Strip(lines[y]), " ")
	if x != ansi.StringWidth(row) {
		t.Fatalf("wrapped caret column %d, text width %d: %q", x, ansi.StringWidth(row), row)
	}
	if x >= 80 {
		t.Fatalf("caret column %d wraps off the line", x)
	}
	if lipglossHeight(m.inputBlockView()) > maxContentLines+2 {
		t.Fatal("input grew past the max")
	}
}

func (m chatModel) inputBlockView() string {
	view, _, _ := m.inputBlock()
	return view
}

func lipglossHeight(s string) int {
	return strings.Count(s, "\n") + 1
}

func TestPlacingWriterParksOnTheCaret(t *testing.T) {
	cur := &frameCursor{}
	cur.set(8, 2, true)
	var buf bytes.Buffer
	w := &placingWriter{out: &buf, cur: cur}
	if _, err := w.Write([]byte("hello\nfooter\r")); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, ansi.CursorUp(2)) || !strings.Contains(got, ansi.CursorForward(8)) {
		t.Fatalf("cursor not moved onto the caret: %q", got)
	}
	if !strings.HasSuffix(got, ansi.ShowCursor) && !strings.Contains(got, ansi.ShowCursor) {
		t.Fatalf("cursor stayed hidden: %q", got)
	}

	buf.Reset()
	if _, err := w.Write([]byte("again\r")); err != nil {
		t.Fatal(err)
	}
	got = buf.String()
	if !strings.HasPrefix(got, ansi.HideCursor+ansi.CursorDown(2)+"\r") {
		t.Fatalf("next frame did not restore the cursor: %q", got)
	}
}

func TestHardwareCursorTracksTheComposer(t *testing.T) {
	m := readyCursorChat()
	assertCursorPrefix(t, m, "> ")

	m.comp.insert("我们额")
	assertCursorPrefix(t, m, "> 我们额")

	m.comp.set("ab")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = next.(chatModel)
	m.comp.insert("我们")
	assertCursorPrefix(t, m, "  我们")

	m = readyCursorChat()
	m.comp.set(strings.Repeat("我们", 40) + " tail")
	body := m.View()
	x, up, show := m.cur.get()
	if !show {
		t.Fatal("cursor hidden")
	}
	plain := strings.TrimRight(ansi.Strip(cursorLine(t, body, up)), " ")
	if ansi.StringWidth(plain) != x || x >= 80 {
		t.Fatalf("wrapped caret column %d line %q", x, plain)
	}

	m.resize(20, 24)
	body = m.View()
	x, up, show = m.cur.get()
	if !show {
		t.Fatal("cursor hidden after resize")
	}
	plain = strings.TrimRight(ansi.Strip(cursorLine(t, body, up)), " ")
	if ansi.StringWidth(plain) != x || x >= 20 {
		t.Fatalf("resized caret column %d line %q", x, plain)
	}

	m = readyCursorChat()
	m.comp.insert("/")
	body = m.View()
	x, up, _ = m.cur.get()
	if x != 3 || displayPrefix(cursorLine(t, body, up), x) != "> /" {
		t.Fatalf("slash caret %d %q", x, displayPrefix(cursorLine(t, body, up), x))
	}
	y := strings.Count(body, "\n") - up
	menu := false
	for i, line := range strings.Split(body, "\n")[:y] {
		if strings.Contains(line, "/reset") && strings.Contains(line, "clear the transcript") {
			menu = true
			if i >= y {
				t.Fatal("menu covered the caret")
			}
		}
	}
	if !menu {
		t.Fatal("slash menu is not above the caret")
	}

	m = readyCursorChat()
	m.busy = true
	m.started = time.Now()
	body = m.View()
	_, up, _ = m.cur.get()
	if got := displayPrefix(cursorLine(t, body, up), 2); got != "> " {
		t.Fatalf("busy caret %q", got)
	}
	if !strings.Contains(ansi.Strip(body), "Working") {
		t.Fatal("working row missing")
	}

	m = readyCursorChat()
	var text strings.Builder
	for i := range 80 {
		text.WriteString("hist-")
		text.WriteString(strconv.Itoa(i))
		text.WriteByte('\n')
	}
	m.addBlock(kindAssistant, text.String())
	m.comp.insert("我们")
	followed := m.View()
	x0, up0, _ := m.cur.get()
	m = m.scrollRows(-m.pageRows())
	scrolled := m.View()
	x1, up1, _ := m.cur.get()
	if x1 != x0 || up1 != up0 {
		t.Fatalf("scroll moved the caret from column %d up %d to column %d up %d", x0, up0, x1, up1)
	}
	if got := displayPrefix(cursorLine(t, scrolled, up1), x1); got != "> 我们" {
		t.Fatalf("scrolled caret %q", got)
	}
	if ansi.Strip(scrolled) == ansi.Strip(followed) {
		t.Fatal("scroll did not change the transcript")
	}
}

func TestPlacingWriterParksOnAltScreenFrames(t *testing.T) {
	cur := &frameCursor{}
	cur.set(8, 3, true)
	var buf bytes.Buffer
	w := &placingWriter{out: &buf, cur: cur}

	if _, err := w.Write([]byte("\x1b[H")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "\x1b[H" {
		t.Fatalf("bare home was treated as a frame: %q", buf.String())
	}

	buf.Reset()
	frame := []byte("one\r\ntwo\r\nthree\r\nfour" + ansi.CursorPosition(0, 4))
	if _, err := w.Write(frame); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "one\r\n") || !strings.Contains(got, ansi.CursorUp(3)) || !strings.Contains(got, ansi.CursorForward(8)) || !strings.HasSuffix(got, ansi.ShowCursor) {
		t.Fatalf("caret not placed: %q", got)
	}

	buf.Reset()
	cur.set(2, 3, true)
	next := []byte("\x1b[Hredrawn\r\nfooter" + ansi.CursorPosition(0, 2))
	if _, err := w.Write(next); err != nil {
		t.Fatal(err)
	}
	got = buf.String()
	if !strings.HasPrefix(got, ansi.HideCursor+"\x1b[H") {
		t.Fatalf("next frame did not hide before painting: %q", got)
	}
	if strings.Contains(got, ansi.CursorDown(3)) {
		t.Fatalf("alt frame stepped through the bottom-left: %q", got)
	}
	if !strings.Contains(got, ansi.CursorForward(2)) || !strings.HasSuffix(got, ansi.ShowCursor) {
		t.Fatalf("second caret: %q", got)
	}

	m := readyCursorChat()
	m.comp.insert("我们额")
	body := m.View()
	x, up, show := m.cur.get()
	if !show {
		t.Fatal("cursor hidden")
	}
	buf.Reset()
	w.cur = m.cur
	lines := strings.Split(body, "\n")
	painted := []byte(strings.Join(lines, "\r\n") + ansi.CursorPosition(0, len(lines)))
	if _, err := w.Write(painted); err != nil {
		t.Fatal(err)
	}
	got = buf.String()
	// The previous frame is still parked, so this one hides first.
	if !strings.Contains(got, ansi.CursorUp(up)) || !strings.Contains(got, ansi.CursorForward(x)) {
		t.Fatalf("live frame ignored caret column %d up %d: %q", x, up, got)
	}
}

func readyCursorChat() chatModel {
	m := newChat(nil, nil)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(80, 24)
	m.cur = &frameCursor{}
	return m
}

func assertCursorPrefix(t *testing.T, m chatModel, want string) {
	t.Helper()
	body := m.View()
	x, up, show := m.cur.get()
	if !show {
		t.Fatal("cursor hidden")
	}
	if got := displayPrefix(cursorLine(t, body, up), x); got != want {
		t.Fatalf("caret prefix %q want %q (column %d, up %d)", got, want, x, up)
	}
	lines := strings.Split(body, "\n")
	foot := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(foot, "ready") && !strings.Contains(foot, "working") {
		t.Fatalf("footer left the bottom: %q", foot)
	}
}

func cursorLine(t *testing.T, body string, up int) string {
	t.Helper()
	lines := strings.Split(body, "\n")
	y := len(lines) - 1 - up
	if y < 0 || y >= len(lines) {
		t.Fatalf("caret row %d of %d", y, len(lines))
	}
	return lines[y]
}

func displayPrefix(line string, width int) string {
	plain := ansi.Strip(line)
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	n := 0
	for _, r := range plain {
		w := ansi.StringWidth(string(r))
		if n+w > width {
			break
		}
		b.WriteRune(r)
		n += w
	}
	return b.String()
}

func TestStyledFooterMatchesPlainWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.ColorProfile()) })
	plain := composeFooter("ready", "glm-5.3", "/tmp/pigo-live", 80)
	styled := styleFooter("ready", "glm-5.3", "/tmp/pigo-live", 80)
	if ansi.Strip(styled) != plain {
		t.Fatalf("styled footer text changed:\nplain  %s\nstyled %s", plain, ansi.Strip(styled))
	}
	if ansi.StringWidth(styled) != ansi.StringWidth(plain) || ansi.StringWidth(styled) > 80 {
		t.Fatalf("styled width %d plain %d", ansi.StringWidth(styled), ansi.StringWidth(plain))
	}
	if styleFooter("ready", "m", "/tmp", 80) == styleFooter("error", "m", "/tmp", 80) {
		t.Fatal("ready and error share one color")
	}
	if styleFooter("working", "m", "/tmp", 80) == styleFooter("ready", "m", "/tmp", 80) {
		t.Fatal("working and ready share one color")
	}
	if !strings.Contains(styled, "00ADD8") && !strings.Contains(strings.ToLower(styled), "38;2;0;173;216") && !strings.Contains(styled, "38;5;") {
		t.Fatalf("model name is not colored: %q", styled)
	}
}
