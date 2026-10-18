package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestWordmarkProportions(t *testing.T) {
	lines := strings.Split(ansi.Strip(wordmark()), "\n")
	if len(lines) < 5 || len(lines) > 6 {
		t.Fatalf("wordmark is %d lines", len(lines))
	}
	width := ansi.StringWidth(lines[0])
	for _, line := range lines[1:] {
		if ansi.StringWidth(line) != width {
			t.Fatalf("wordmark line width %d, want %d", ansi.StringWidth(line), width)
		}
	}
	if width < 50 || width > 70 {
		t.Fatalf("wordmark is %d cols", width)
	}
}

func TestSessionCardAndCellGaps(t *testing.T) {
	m := newChat(nil, nil)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(80, 24)
	m.addBlock(kindUser, "hello")
	m.addBlock(kindAssistant, "world")
	m.addBlock(kindInfo, "Worked for 17s")
	m.addBlock(kindTool, "• Ran ls")
	m.addBlock(kindUser, "again")

	got := m.transcript()
	plain := ansi.Strip(got)
	if strings.Contains(plain, "\n\n\n") {
		t.Fatalf("cells have more than one blank line:\n%s", plain)
	}
	for _, want := range []string{"pigo 0.1.0", "glm-5.3", "/tmp/pigo-live", "╭", "hello", "world", "Worked for 17s", "Ran ls", "again"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("transcript missing %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(plain, "██████\n\n╭") {
		t.Fatal("wordmark is glued to the session card")
	}
	if !strings.Contains(plain, "• world\n\n─ Worked for 17s") {
		t.Fatalf("assistant cell is glued to the worked rule:\n%s", plain)
	}

	rule := ansi.Strip(m.workedRule("Worked for 17s"))
	if ansi.StringWidth(rule) != 80 || !strings.HasPrefix(rule, "─ Worked for 17s ") {
		t.Fatalf("rule: width %d %q", ansi.StringWidth(rule), rule)
	}
	wide := newChat(nil, nil)
	wide.resize(140, 40)
	if w := ansi.StringWidth(ansi.Strip(wide.workedRule("Worked for 3s"))); w != 140 {
		t.Fatalf("wide rule is %d cols", w)
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.ColorProfile()) })
	user := m.renderBlock(block{kind: kindUser, text: "hello"})
	lines := strings.Split(user, "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], "> hello") {
		t.Fatalf("user block: %#v", lines)
	}
	if !strings.Contains(lines[0], "\x1b") || !strings.Contains(lines[2], "\x1b") {
		t.Fatal("user pad rows lost the gray bar")
	}
	parts := strings.Split(got, "\n")
	gap := ""
	for i, line := range parts {
		if strings.Contains(ansi.Strip(line), "> hello") && i+2 < len(parts) {
			gap = parts[i+2]
			break
		}
	}
	if gap != "" {
		t.Fatalf("gap under the user block is not a plain blank line: %q", gap)
	}

	for _, width := range []int{80, 140} {
		card := wideCard(t, width)
		for _, line := range strings.Split(card, "\n") {
			if w := ansi.StringWidth(line); w != width {
				t.Fatalf("card line at %d is %d cols: %q", width, w, ansi.Strip(line))
			}
		}
	}
}

func wideCard(t *testing.T, width int) string {
	t.Helper()
	m := newChat(nil, nil)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(width, 40)
	return m.sessionCard()
}

func TestComposerGapKeepsTheCaret(t *testing.T) {
	for _, width := range []int{80, 140} {
		m := newChat(nil, nil)
		m.modelName = "glm-5.3"
		m.root = "/tmp/pigo-live"
		m.resize(width, 24)
		m.comp.insert("我们额")
		body, x, y := m.layout()
		lines := strings.Split(body, "\n")
		if x != 2+ansi.StringWidth("我们额") {
			t.Fatalf("width %d caret column %d", width, x)
		}
		if !strings.Contains(ansi.Strip(lines[y]), "我们额") {
			t.Fatalf("width %d caret row %d: %q", width, y, lines[y])
		}
		if y+2 >= len(lines) || lines[y+2] != "" {
			t.Fatalf("width %d missing the blank line under the composer", width)
		}
		if ansi.Strip(lines[len(lines)-1]) == "" || !strings.Contains(ansi.Strip(lines[len(lines)-1]), "ready") {
			t.Fatalf("width %d footer: %q", width, lines[len(lines)-1])
		}
		up := strings.Count(body, "\n") - y
		if up != 3 {
			t.Fatalf("width %d up=%d", width, up)
		}
		assertWidth(t, body, width)
	}
}
