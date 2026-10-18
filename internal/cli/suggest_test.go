package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zyy17/pigo/internal/skills"
)

func TestFilterAndCompleteCommands(t *testing.T) {
	list := []skills.Skill{
		{Name: "explain-go", Description: "Explain Go code"},
		{Name: "review", Description: "Review a change"},
	}
	cmds := slashCommands(list)
	if len(cmds) != 4 || cmds[0].Name != "/reset" || cmds[2].Name != "/skill:explain-go" {
		t.Fatalf("commands: %#v", cmds)
	}

	all := filterCommands(cmds, "/")
	if len(all) != 4 {
		t.Fatalf("/ matches %d", len(all))
	}
	reset := filterCommands(cmds, "/R")
	if len(reset) != 1 || reset[0].Name != "/reset" {
		t.Fatalf("/R: %#v", reset)
	}
	skillsOnly := filterCommands(cmds, "/skill:ex")
	if len(skillsOnly) != 1 || skillsOnly[0].Desc != "Explain Go code" {
		t.Fatalf("skill filter: %#v", skillsOnly)
	}
	if partialCommand("/r", reset) != true {
		t.Fatal("/r should complete")
	}
	if partialCommand("/reset", reset) {
		t.Fatal("exact /reset should send")
	}
	if partialCommand("/skill:explain-go please", filterCommands(cmds, "/skill:explain-go please")) {
		t.Fatal("a request should send")
	}

	m := newChat(nil, nil)
	m.resize(80, 24)
	m.skills = list
	m.comp.insert("/s")
	m.noteTyped()
	if !m.menuOpen() {
		t.Fatal("menu closed")
	}
	m.moveSuggest(1)
	m.completeSuggest()
	if m.comp.text() != "/skill:review" {
		t.Fatalf("completion: %q", m.comp.text())
	}

	m.comp.set("/r")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(chatModel)
	if m.comp.text() != "/reset" || m.quitting {
		t.Fatalf("partial enter: %q quit=%v", m.comp.text(), m.quitting)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(chatModel)
	if !strings.Contains(m.transcript(), "transcript cleared") {
		t.Fatal("second enter did not reset")
	}
	if cmd == nil {
		t.Fatal("missing reset print")
	}

	m.comp.set("/e")
	m.noteTyped()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(chatModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(chatModel)
	if m.comp.text() != "/e" {
		t.Fatalf("esc should close the menu, got %q", m.comp.text())
	}

	m.comp.set("hello")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = next.(chatModel)
	if m.comp.text() != "hello\n" || m.busy {
		t.Fatalf("alt+enter: %q busy=%v", m.comp.text(), m.busy)
	}
}

func TestSuggestSitsAboveTheCaret(t *testing.T) {
	m := newChat(nil, nil)
	m.resize(80, 24)
	_, _, y0 := m.layout()
	m.comp.insert("/")
	body, x, y := m.layout()
	if y <= y0 {
		t.Fatalf("menu did not push the caret down: %d -> %d", y0, y)
	}
	if x != 3 {
		t.Fatalf("caret after '> /' at column %d", x)
	}
	if !strings.Contains(body, "/reset") || !strings.Contains(body, "clear the transcript") {
		t.Fatalf("menu missing:\n%s", body)
	}
}
