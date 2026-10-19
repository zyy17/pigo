package cli

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zyy17/pigo/internal/agent"
)

func TestTypingWhileBusyEditsTheComposer(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.busy = true

	next, _ := m.Update(tea.KeyPressMsg{Text: "ab", Code: 'a'})
	m = next.(chatModel)
	if m.comp.text() != "ab" || !m.busy {
		t.Fatalf("typed %q busy %v", m.comp.text(), m.busy)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(chatModel)
	if m.comp.text() != "a" {
		t.Fatalf("backspace %q", m.comp.text())
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = next.(chatModel)
	if m.comp.text() != "a\n" || m.queued != "" {
		t.Fatalf("shift+enter %q queued %q", m.comp.text(), m.queued)
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = next.(chatModel)
	if cmd != nil || m.quitting {
		t.Fatal("ctrl+d while busy quit")
	}
}

func TestPasteWhileBusyInserts(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.busy = true
	m.comp.set("go")

	next, _ := m.Update(tea.PasteMsg{Content: "pher"})
	m = next.(chatModel)
	if m.comp.text() != "gopher" || !m.busy {
		t.Fatalf("paste %q busy %v", m.comp.text(), m.busy)
	}
}

func TestEscWhileBusyAborts(t *testing.T) {
	started := make(chan struct{})
	session := blockingSession(t, started)
	events := make(chan agent.Event, 64)
	session.Subscribe(func(ev agent.Event) { events <- ev })
	m := newChat(session)
	m.resize(80, 24)
	m.comp.set("run")

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	done := make(chan chatModel, 1)
	go func() {
		done <- runCmds(t, m, cmd, events)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not start")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	select {
	case finished := <-done:
		if finished.busy || finished.status != "interrupted" {
			t.Fatalf("status %q busy %v", finished.status, finished.busy)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("esc did not interrupt")
	}
}

func TestEnterWhileBusyQueuesUntilAgentEnd(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.busy = true
	m.comp.set("later")

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if cmd != nil || m.comp.text() != "" || m.queued != "later" || !m.busy {
		t.Fatalf("queue %q composer %q busy %v cmd %v", m.queued, m.comp.text(), m.busy, cmd != nil)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "↳ queued: later") {
		t.Fatalf("queued line missing:\n%s", view)
	}

	m.comp.set("more")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if m.queued != "later\nmore" {
		t.Fatalf("second enter %q", m.queued)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "↳ queued: later more") {
		t.Fatal("queued line did not stay one line")
	}

	next, cmd = m.Update(streamMsg{event: agent.Event{Type: agent.AgentEnd}})
	m = next.(chatModel)
	if cmd == nil || !m.busy || m.queued != "" || m.comp.text() != "" {
		t.Fatalf("after end busy %v queued %q composer %q cmd %v", m.busy, m.queued, m.comp.text(), cmd != nil)
	}
	sent := ansi.Strip(m.transcript())
	if !strings.Contains(sent, "> later") || !strings.Contains(sent, "more") {
		t.Fatalf("queued text was not submitted:\n%s", sent)
	}
	if strings.Contains(ansi.Strip(m.View().Content), "↳ queued:") {
		t.Fatal("queued line stayed after submit")
	}
}

func TestQueuedTurnKeepsTheDraft(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.busy = true
	m.comp.set("later")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	m.comp.set("half-typed draft")

	next, cmd := m.Update(streamMsg{event: agent.Event{Type: agent.AgentEnd}})
	m = next.(chatModel)
	if cmd == nil || !m.busy || m.queued != "" || m.comp.text() != "half-typed draft" {
		t.Fatalf("draft %q busy %v queued %q cmd %v", m.comp.text(), m.busy, m.queued, cmd != nil)
	}
	if !strings.Contains(ansi.Strip(m.transcript()), "> later") {
		t.Fatal("queue was not sent")
	}
}

func TestEscBeforeAgentEndRestoresTheQueue(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.busy = true
	m.comp.set("later")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	m.comp.set("draft")

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(chatModel)
	next, cmd := m.Update(streamMsg{event: agent.Event{Type: agent.AgentEnd}})
	m = next.(chatModel)
	if cmd != nil || m.busy || m.queued != "" || m.comp.text() != "later\ndraft" {
		t.Fatalf("restored %q busy %v queued %q cmd %v", m.comp.text(), m.busy, m.queued, cmd != nil)
	}
	if strings.Contains(ansi.Strip(m.transcript()), "> later") {
		t.Fatal("queue was sent after Esc")
	}
}

func TestQueuedUnknownSkillReturnsToTheComposer(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.busy = true
	m.comp.set("/skill:nope do it")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	m.comp.set("draft")

	next, cmd := m.Update(streamMsg{event: agent.Event{Type: agent.AgentEnd}})
	m = next.(chatModel)
	if cmd != nil || m.busy || m.comp.text() != "/skill:nope do it\ndraft" {
		t.Fatalf("composer %q busy %v cmd %v", m.comp.text(), m.busy, cmd != nil)
	}
	if !strings.Contains(m.transcript(), "unknown skill") {
		t.Fatal("skill error was not shown")
	}
}

func TestInterruptedTurnRestoresTheQueue(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.busy = true
	m.comp.set("later")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	m.comp.set("draft")

	next, cmd := m.Update(streamMsg{event: agent.Event{Type: agent.AgentEnd, Err: agent.ErrInterrupted}})
	m = next.(chatModel)
	if cmd != nil || m.busy || m.queued != "" || m.comp.text() != "later\ndraft" {
		t.Fatalf("restored %q busy %v queued %q", m.comp.text(), m.busy, m.queued)
	}
	if m.status != "interrupted" {
		t.Fatalf("status %q", m.status)
	}
	if strings.Contains(ansi.Strip(m.transcript()), "> later") {
		t.Fatal("interrupted queue was sent")
	}
}

func TestBusySlashCommands(t *testing.T) {
	m := testChat(t)
	m.resize(80, 24)
	m.busy = true
	m.comp.set("/skill:demo hi")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if m.queued != "/skill:demo hi" {
		t.Fatalf("skill queued %q", m.queued)
	}

	m.comp.set("/reset")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if cmd != nil || m.queued != "/skill:demo hi" || m.comp.text() != "" || !m.busy {
		t.Fatalf("reset queued %q composer %q busy %v", m.queued, m.comp.text(), m.busy)
	}
	if !strings.Contains(m.transcript(), "/reset waits") {
		t.Fatal("reset was not refused")
	}

	m.comp.set("/exit")
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if cmd == nil || !m.quitting {
		t.Fatal("exit did not quit")
	}

	for _, line := range []string{"exit", "quit"} {
		q := testChat(t)
		q.resize(80, 24)
		q.busy = true
		q.comp.set(line)
		next, cmd = q.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		q = next.(chatModel)
		if cmd == nil || !q.quitting {
			t.Fatalf("%s did not quit", line)
		}
	}
}
