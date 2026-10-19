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
	if cmd != nil {
		t.Fatal("reset printed into the terminal")
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
	m.comp.insert("/")
	body, x, y := m.layout()
	if x != 3 {
		t.Fatalf("caret after '> /' at column %d", x)
	}
	lines := strings.Split(body, "\n")
	menuAt := -1
	for i, line := range lines {
		if strings.Contains(line, "/reset") && strings.Contains(line, "clear the transcript") {
			menuAt = i
			break
		}
	}
	if menuAt < 0 || menuAt >= y {
		t.Fatalf("menu at %d, caret at %d\n%s", menuAt, y, body)
	}
	if !strings.Contains(lines[y], "> /") {
		t.Fatalf("caret row: %q", lines[y])
	}
}
