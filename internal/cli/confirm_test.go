package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestConfirmPrompt(t *testing.T) {
	m := readyChat()
	m.busy = true
	reply := make(chan bool, 1)
	m.ask = &confirmReq{
		command: "rm -rf /",
		reason:  "recursive delete of / or home",
		reply:   reply,
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"! allow this command?", "rm -rf /", "recursive delete of / or home", "enter", "esc"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "esc to interrupt") {
		t.Fatalf("working row still shown:\n%s", view)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if <-reply {
		t.Fatal("esc allowed the command")
	}
	if next.(chatModel).ask != nil {
		t.Fatal("confirm still open")
	}
}

func TestConfirmKeys(t *testing.T) {
	cases := []struct {
		name  string
		key   tea.KeyMsg
		allow bool
	}{
		{name: "enter", key: tea.KeyMsg{Type: tea.KeyEnter}, allow: true},
		{name: "y", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}, allow: true},
		{name: "n", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}, allow: false},
		{name: "ctrl-c", key: tea.KeyMsg{Type: tea.KeyCtrlC}, allow: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := readyChat()
			m.busy = true
			reply := make(chan bool, 1)
			m.ask = &confirmReq{command: "sudo true", reason: "sudo", reply: reply}
			next, _ := m.Update(tc.key)
			if got := <-reply; got != tc.allow {
				t.Fatalf("allow %v", got)
			}
			if next.(chatModel).ask != nil {
				t.Fatal("confirm still open")
			}
		})
	}
}

func readyChat() chatModel {
	m := newChat(nil, nil)
	m.resize(80, 24)
	return m
}
