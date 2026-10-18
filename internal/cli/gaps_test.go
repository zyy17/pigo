package cli

import (
	"bytes"
	"errors"
	"io"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComposerEdits(t *testing.T) {
	m := newChat(nil, nil)
	m.resize(80, 24)
	m.comp.set("ab cd")
	steps := []struct {
		key  tea.KeyType
		want string
	}{
		{tea.KeyLeft, "ab cd"},
		{tea.KeyBackspace, "ab d"},
		{tea.KeyHome, "ab d"},
		{tea.KeyDelete, "b d"},
		{tea.KeyEnd, "b d"},
		{tea.KeyRight, "b d"},
	}
	for _, step := range steps {
		next, _ := m.Update(tea.KeyMsg{Type: step.key})
		m = next.(chatModel)
		if m.comp.text() != step.want {
			t.Fatalf("after %v: %q", step.key, m.comp.text())
		}
	}
	m.comp.set("ab cd")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = next.(chatModel)
	if m.comp.text() != "ab " {
		t.Fatalf("word back %q", m.comp.text())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = next.(chatModel)
	if m.comp.text() != "" {
		t.Fatalf("clear %q", m.comp.text())
	}
}

func TestWindowAndLayoutEdges(t *testing.T) {
	if windowStart(10, 0, 4) != 0 || windowStart(3, 5, 2) != 0 {
		t.Fatal("window starts at zero when it all fits")
	}
	if windowStart(10, 3, 9) != 7 || windowStart(10, 3, 0) != 0 {
		t.Fatalf("window %d %d", windowStart(10, 3, 9), windowStart(10, 3, 0))
	}
	if paneCols(0) != 1 || textCols(2) != 1 || textCols(80) != 78 {
		t.Fatalf("cols %d %d %d", paneCols(0), textCols(2), textCols(80))
	}
	got, dropped := fitHeightDrop("a\nb\nc", 2)
	if got != "b\nc" || dropped != 1 {
		t.Fatalf("drop %q %d", got, dropped)
	}
	if s, n := fitHeightDrop("", 2); s != "" || n != 0 {
		t.Fatalf("empty drop %q %d", s, n)
	}
	if start, end := suggestRange(3, 1, 5); start != 0 || end != 3 {
		t.Fatalf("range %d %d", start, end)
	}
	if start, end := suggestRange(10, 9, 4); start != 6 || end != 10 {
		t.Fatalf("tail range %d %d", start, end)
	}
	if caretColumn("> ", []rune("ab"), 9) != 4 {
		t.Fatal(caretColumn("> ", []rune("ab"), 9))
	}
}

func TestCursorFileShim(t *testing.T) {
	w := &placingWriter{}
	if w.Fd() != 0 || w.Close() != nil {
		t.Fatal("shim")
	}
	n, err := w.Read(make([]byte, 1))
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read %d %v", n, err)
	}
	var buf bytes.Buffer
	w.out = &buf
	if w.dest() != &buf {
		t.Fatal("out")
	}
}
