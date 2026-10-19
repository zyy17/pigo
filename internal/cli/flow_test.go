package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/coding"
	"github.com/zyy17/pigo/internal/e2etest"
)

func TestHeadlessChatCompletesATurn(t *testing.T) {
	dir := t.TempDir()
	skill := filepath.Join(dir, ".pi", "skills", "greet")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "---\nname: greet\ndescription: Write a greeting.\n---\n\nSay hello.\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	_, srv := e2etest.NewServer(t, "test-key", []e2etest.Reply{
		{Calls: []e2etest.Call{{
			ID: "call_write", Name: "write_file",
			Args: `{"path":"hello.txt","content":"hi\n"}`,
		}}},
		{Text: "saved"},
	})
	session, err := coding.New(coding.Config{
		APIKey:        "test-key",
		BaseURL:       srv.URL + "/v1",
		Model:         "test-model",
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan agent.Event, 64)
	session.Subscribe(func(ev agent.Event) { events <- ev })
	m := newChat(session)
	m.resize(80, 24)

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = next.(chatModel)
	if m.comp.text() != "\n" {
		t.Fatalf("shift+enter %q", m.comp.text())
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	m = next.(chatModel)
	if m.comp.text() != "\n\n" {
		t.Fatalf("ctrl+j %q", m.comp.text())
	}
	m.comp.set("please write hello.txt")

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if !m.busy || !strings.Contains(ansi.Strip(m.View().Content), "Working") {
		t.Fatalf("working view:\n%s", ansi.Strip(m.View().Content))
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "please write hello.txt") || !strings.Contains(ansi.Strip(m.View().Content), askPlaceholder) {
		t.Fatal("submitted line is not in the transcript above a cleared composer")
	}
	m = runCmds(t, m, cmd, events)
	if m.busy || m.status != "ready" {
		t.Fatalf("status %q busy %v", m.status, m.busy)
	}
	plain := ansi.Strip(m.transcript())
	for _, want := range []string{"please write hello.txt", "Ran write_file", "saved", "> "} {
		if !strings.Contains(plain, want) {
			t.Fatalf("transcript missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "\n\n\n") {
		t.Fatalf("transcript doubled a blank line:\n%s", plain)
	}
	body, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil || string(body) != "hi\n" {
		t.Fatalf("hello.txt %q err %v", body, err)
	}
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	footer := lines[len(lines)-1]
	if !strings.Contains(footer, "ready") || strings.Contains(footer, "saved") {
		t.Fatalf("footer after the turn:\n%s", footer)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "saved") {
		t.Fatal("finished reply left the frame")
	}
	_, x, y := m.layout()
	lines = strings.Split(m.View().Content, "\n")
	if x != 2 || !strings.Contains(ansi.Strip(lines[y]), askPlaceholder) {
		t.Fatalf("caret x=%d y=%d", x, y)
	}
	if up := strings.Count(m.View().Content, "\n") - y; up != 3 {
		t.Fatalf("up=%d", up)
	}

	next, _ = m.Update(tea.KeyPressMsg{Text: "/sk", Code: '/'})
	m = next.(chatModel)
	menu := ansi.Strip(m.View().Content)
	if !strings.Contains(menu, "/skill:greet") || !strings.Contains(menu, "Write a greeting") || strings.Contains(menu, "clear the transcript") {
		t.Fatalf("slash menu:\n%s", menu)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if m.busy || m.comp.text() != "/skill:greet" {
		t.Fatalf("enter completed %q busy %v", m.comp.text(), m.busy)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(chatModel)
	if strings.Contains(ansi.Strip(m.View().Content), "Write a greeting") {
		t.Fatal("esc left the slash menu open")
	}
}

func TestHeadlessEscInterruptsAHang(t *testing.T) {
	dir := t.TempDir()
	script, srv := e2etest.NewServer(t, "test-key", []e2etest.Reply{{Hang: true}})
	session, err := coding.New(coding.Config{
		APIKey:        "test-key",
		BaseURL:       srv.URL + "/v1",
		Model:         "test-model",
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan agent.Event, 64)
	session.Subscribe(func(ev agent.Event) { events <- ev })
	m := newChat(session)
	m.resize(80, 24)
	m.comp.set("wait")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)

	done := make(chan chatModel, 1)
	go func() {
		done <- runCmds(t, m, cmd, events)
	}()
	script.Wait(t)
	if _, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); cmd != nil {
		t.Fatal("esc started a command")
	}
	select {
	case finished := <-done:
		if finished.busy || finished.status != "interrupted" {
			t.Fatalf("status %q busy %v", finished.status, finished.busy)
		}
		if !strings.Contains(ansi.Strip(finished.View().Content), "interrupted") {
			t.Fatalf("footer:\n%s", ansi.Strip(finished.View().Content))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("esc did not interrupt")
	}
}
