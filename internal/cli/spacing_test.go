package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestWordmarkProportions(t *testing.T) {
	lines := strings.Split(ansi.Strip(wordmark()), "\n")
	if len(lines) != 4 {
		t.Fatalf("wordmark is %d lines", len(lines))
	}
	width := ansi.StringWidth(lines[0])
	for _, line := range lines[1:] {
		if ansi.StringWidth(line) != width {
			t.Fatalf("wordmark line width %d, want %d", ansi.StringWidth(line), width)
		}
	}
	if width < 32 || width > 38 {
		t.Fatalf("wordmark is %d cols", width)
	}
}

func TestSessionCardAndCellGaps(t *testing.T) {
	m := testChat(t)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(80, 24)
	m.addBlock(kindUser, "hello")
	m.addBlock(kindAssistant, "world")
	m.addBlock(kindWorked, "Worked for 17s")
	m.addBlock(kindRan, "• Ran ls")
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
	if !strings.HasPrefix(plain, "\n█") || strings.HasPrefix(plain, "\n\n") {
		t.Fatal("wordmark should have one blank line above it")
	}
	short := testChat(t)
	short.resize(80, 10)
	if strings.HasPrefix(ansi.Strip(short.opening()), "\n") {
		t.Fatal("a short terminal still leads with a blank wordmark line")
	}
	if !strings.Contains(plain, "• world\n\n─ Worked for 17s") {
		t.Fatalf("assistant cell is glued to the worked rule:\n%s", plain)
	}

	rule := ansi.Strip(m.workedRule("Worked for 17s"))
	if ansi.StringWidth(rule) != 80 || !strings.HasPrefix(rule, "─ Worked for 17s ") {
		t.Fatalf("rule: width %d %q", ansi.StringWidth(rule), rule)
	}
	wide := testChat(t)
	wide.resize(140, 40)
	if w := ansi.StringWidth(ansi.Strip(wide.workedRule("Worked for 3s"))); w != 140 {
		t.Fatalf("wide rule is %d cols", w)
	}

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
	m := testChat(t)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(width, 40)
	return m.sessionCard()
}

func TestComposerGapKeepsTheCaret(t *testing.T) {
	for _, width := range []int{80, 140} {
		m := testChat(t)
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

func TestWorkingRowBreathingRoom(t *testing.T) {
	for _, width := range []int{80, 110} {
		m := testChat(t)
		m.modelName = "glm-5.3"
		m.root = "/tmp/pigo-live"
		m.resize(width, 24)
		m.busy = true
		m.started = time.Now()

		body, x, y := m.layout()
		lines := strings.Split(body, "\n")
		if len(lines) != 24 {
			t.Fatalf("width %d frame is %d lines", width, len(lines))
		}
		wi := workingLine(t, lines)
		if lines[wi+1] != "" {
			t.Fatalf("width %d missing the blank under the working row: %q", width, lines[wi+1])
		}
		if wi+1 >= len(lines)-2 {
			t.Fatalf("width %d working row is not above the composer", width)
		}
		assertCaret(t, lines, x, y, body)

		m.live = "streaming reply"
		body, x, y = m.layout()
		lines = strings.Split(body, "\n")
		plain := ansi.Strip(body)
		if strings.Contains(ansi.Strip(strings.Join(m.historyLines(), "\n")), "\n\n\n") {
			t.Fatalf("width %d doubled a blank inside the transcript:\n%s", width, plain)
		}
		wi = workingLine(t, lines)
		liveAt := strings.Index(plain, "streaming reply")
		workAt := strings.Index(plain, "Working")
		if liveAt < 0 || workAt < 0 || liveAt > workAt {
			t.Fatalf("width %d live text is not above the working row:\n%s", width, plain)
		}
		if lines[wi+1] != "" {
			t.Fatalf("width %d missing the blank under the working row while streaming", width)
		}
		assertCaret(t, lines, x, y, body)
		if h := lipglossHeight(body); h > 24 {
			t.Fatalf("width %d working frame is %d lines", width, h)
		}
	}

	m := testChat(t)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(40, 8)
	m.busy = true
	m.started = time.Now()
	body, x, y := m.layout()
	lines := strings.Split(body, "\n")
	if len(lines) > 8 {
		t.Fatalf("short frame is %d lines", len(lines))
	}
	if workingLine(t, lines) < 0 {
		t.Fatal("short frame clipped the working row")
	}
	assertCaret(t, lines, x, y, body)
}

func workingLine(t *testing.T, lines []string) int {
	t.Helper()
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), "Working") {
			return i
		}
	}
	t.Fatal("working row missing")
	return -1
}

func assertCaret(t *testing.T, lines []string, x, y int, body string) {
	t.Helper()
	if x != 2 || y < 0 || y >= len(lines) || !strings.Contains(ansi.Strip(lines[y]), askPlaceholder) {
		t.Fatalf("caret x=%d y=%d", x, y)
	}
	if up := strings.Count(body, "\n") - y; up != 3 {
		t.Fatalf("up=%d, want the pad, the blank line, and the footer", up)
	}
}
