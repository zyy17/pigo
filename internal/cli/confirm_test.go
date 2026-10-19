package cli

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestConfirmPrompt(t *testing.T) {
	m := readyChat(t)
	m.busy = true
	reply := make(chan bool, 1)
	m.ask = &confirmReq{
		title:  "! allow this command?",
		body:   "rm -rf /",
		detail: "recursive delete of / or home",
		reply:  reply,
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"! allow this command?", "rm -rf /", "recursive delete of / or home", "enter", "esc"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "Working") {
		t.Fatalf("working row still shown:\n%s", view)
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
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
		key   tea.KeyPressMsg
		allow bool
	}{
		{name: "enter", key: tea.KeyPressMsg{Code: tea.KeyEnter}, allow: true},
		{name: "y", key: tea.KeyPressMsg{Text: "y", Code: 'y'}, allow: true},
		{name: "n", key: tea.KeyPressMsg{Text: "n", Code: 'n'}, allow: false},
		{name: "ctrl-c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, allow: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := readyChat(t)
			m.busy = true
			reply := make(chan bool, 1)
			m.ask = &confirmReq{title: "! allow this command?", body: "sudo true", detail: "sudo", reply: reply}
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

func readyChat(t *testing.T) chatModel {
	t.Helper()
	m := testChat(t)
	m.resize(80, 24)
	return m
}
