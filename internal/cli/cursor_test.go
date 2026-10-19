package cli

import (
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCaretColumnForCJKAndWrap(t *testing.T) {
	m := testChat(t)
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
	lines = strings.Split(m.View().Content, "\n")
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

func TestCursorTracksTheComposer(t *testing.T) {
	m := readyCursorChat(t)
	assertCursorPrefix(t, m, "> ")

	m.comp.insert("我们额")
	assertCursorPrefix(t, m, "> 我们额")

	m.comp.set("ab")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = next.(chatModel)
	m.comp.insert("我们")
	assertCursorPrefix(t, m, "  我们")

	m = readyCursorChat(t)
	m.comp.set(strings.Repeat("我们", 40) + " tail")
	v := m.View()
	plain := strings.TrimRight(ansi.Strip(cursorLine(t, v.Content, v.Cursor.Y)), " ")
	if ansi.StringWidth(plain) != v.Cursor.X || v.Cursor.X >= 80 {
		t.Fatalf("wrapped caret column %d line %q", v.Cursor.X, plain)
	}

	m.resize(20, 24)
	v = m.View()
	plain = strings.TrimRight(ansi.Strip(cursorLine(t, v.Content, v.Cursor.Y)), " ")
	if v.Cursor == nil || ansi.StringWidth(plain) != v.Cursor.X || v.Cursor.X >= 20 {
		t.Fatalf("resized caret column %d line %q", v.Cursor.X, plain)
	}

	m = readyCursorChat(t)
	m.comp.insert("/")
	v = m.View()
	if v.Cursor.X != 3 || displayPrefix(cursorLine(t, v.Content, v.Cursor.Y), v.Cursor.X) != "> /" {
		t.Fatalf("slash caret %d %q", v.Cursor.X, displayPrefix(cursorLine(t, v.Content, v.Cursor.Y), v.Cursor.X))
	}
	menu := false
	for i, line := range strings.Split(v.Content, "\n")[:v.Cursor.Y] {
		if strings.Contains(line, "/reset") && strings.Contains(line, "clear the transcript") {
			menu = true
			if i >= v.Cursor.Y {
				t.Fatal("menu covered the caret")
			}
		}
	}
	if !menu {
		t.Fatal("slash menu is not above the caret")
	}

	m = readyCursorChat(t)
	m.busy = true
	m.started = time.Now()
	v = m.View()
	if got := displayPrefix(cursorLine(t, v.Content, v.Cursor.Y), 2); got != "> " {
		t.Fatalf("busy caret %q", got)
	}
	if !strings.Contains(ansi.Strip(v.Content), "Working") {
		t.Fatal("working row missing")
	}

	m = readyCursorChat(t)
	var text strings.Builder
	for i := range 80 {
		text.WriteString("hist-")
		text.WriteString(strconv.Itoa(i))
		text.WriteByte('\n')
	}
	m.addBlock(kindAssistant, text.String())
	m.comp.insert("我们")
	followed := m.View()
	m.scrollRows(-m.pageRows())
	scrolled := m.View()
	if scrolled.Cursor.X != followed.Cursor.X || scrolled.Cursor.Y != followed.Cursor.Y {
		t.Fatalf("scroll moved the caret from %d,%d to %d,%d", followed.Cursor.X, followed.Cursor.Y, scrolled.Cursor.X, scrolled.Cursor.Y)
	}
	if got := displayPrefix(cursorLine(t, scrolled.Content, scrolled.Cursor.Y), scrolled.Cursor.X); got != "> 我们" {
		t.Fatalf("scrolled caret %q", got)
	}
	if ansi.Strip(scrolled.Content) == ansi.Strip(followed.Content) {
		t.Fatal("scroll did not change the transcript")
	}
}

func readyCursorChat(t *testing.T) chatModel {
	t.Helper()
	m := testChat(t)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(80, 24)
	return m
}

func assertCursorPrefix(t *testing.T, m chatModel, want string) {
	t.Helper()
	v := m.View()
	if v.Cursor == nil {
		t.Fatal("cursor hidden")
	}
	if !v.AltScreen || v.MouseMode != tea.MouseModeCellMotion {
		t.Fatal("frame left the alternate screen or dropped the mouse")
	}
	if got := displayPrefix(cursorLine(t, v.Content, v.Cursor.Y), v.Cursor.X); got != want {
		t.Fatalf("caret prefix %q want %q (column %d, row %d)", got, want, v.Cursor.X, v.Cursor.Y)
	}
	lines := strings.Split(v.Content, "\n")
	foot := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(foot, "ready") && !strings.Contains(foot, "working") {
		t.Fatalf("footer left the bottom: %q", foot)
	}
}

func cursorLine(t *testing.T, body string, y int) string {
	t.Helper()
	lines := strings.Split(body, "\n")
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
	styled := styleFooter("ready", "glm-5.3", "/tmp/pigo-live", "", 80)
	plain := ansi.Strip(styled)
	if ansi.StringWidth(styled) != ansi.StringWidth(plain) || ansi.StringWidth(styled) > 80 {
		t.Fatalf("styled width %d plain %d", ansi.StringWidth(styled), ansi.StringWidth(plain))
	}
	for _, want := range []string{"glm-5.3", "/exit"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("footer missing %q: %s", want, plain)
		}
	}
	if strings.Contains(plain, "esc to interrupt") {
		t.Fatalf("idle footer offers interrupt: %s", plain)
	}
	if styleFooter("ready", "m", "/tmp", "", 80) == styleFooter("error", "m", "/tmp", "", 80) {
		t.Fatal("ready and error share one color")
	}
	if styleFooter("working", "m", "/tmp", "", 80) == styleFooter("ready", "m", "/tmp", "", 80) {
		t.Fatal("working and ready share one color")
	}
	if !strings.Contains(styled, "00ADD8") && !strings.Contains(strings.ToLower(styled), "38;2;0;173;216") && !strings.Contains(styled, "38;5;") {
		t.Fatalf("model name is not colored: %q", styled)
	}
}
