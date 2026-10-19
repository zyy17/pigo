package cli

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestComposerEdits(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.comp.set("ab cd")
	steps := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{tea.KeyPressMsg{Code: tea.KeyLeft}, "ab cd"},
		{tea.KeyPressMsg{Code: tea.KeyBackspace}, "ab d"},
		{tea.KeyPressMsg{Code: tea.KeyHome}, "ab d"},
		{tea.KeyPressMsg{Code: tea.KeyDelete}, "b d"},
		{tea.KeyPressMsg{Code: tea.KeyEnd}, "b d"},
		{tea.KeyPressMsg{Code: tea.KeyRight}, "b d"},
	}
	for _, step := range steps {
		next, _ := m.Update(step.key)
		m = next.(chatModel)
		if m.comp.text() != step.want {
			t.Fatalf("after %v: %q", step.key, m.comp.text())
		}
	}
	m.comp.set("ab cd")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	m = next.(chatModel)
	if m.comp.text() != "ab " {
		t.Fatalf("word back %q", m.comp.text())
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
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
	if textCols(2) != 1 || textCols(80) != 78 {
		t.Fatalf("cols %d %d", textCols(2), textCols(80))
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
