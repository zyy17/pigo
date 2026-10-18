package cli

import (
	"bytes"
	"strings"
	"testing"

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
