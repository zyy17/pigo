package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
)

type csiMsg []byte

func TestShiftEnterSequences(t *testing.T) {
	for _, seq := range []string{"\x1b[27;2;13~", "\x1b[13;2u", "\x1b[13;2;13u", "\x1b[27;6;13~"} {
		if !isShiftEnter(csiMsg(seq)) {
			t.Fatalf("shift+enter not recognized: %q", seq)
		}
	}
	for _, seq := range []string{"\x1b[27;3;13~", "\x1b[27;2;65~", "\x1b[13;3u", "\r"} {
		if isShiftEnter(csiMsg(seq)) {
			t.Fatalf("not shift+enter: %q", seq)
		}
	}
	if isShiftEnter(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("plain enter looked like shift+enter")
	}
	if _, ok := rewriteKeys(nil, csiMsg("\x1b[27;2;13~")).(shiftEnterMsg); !ok {
		t.Fatal("filter did not rewrite shift+enter")
	}
}

func TestNewlineBindings(t *testing.T) {
	m := newChat(nil, nil)
	m.resize(80, 24)
	m.comp.insert("hello")

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = next.(chatModel)
	if m.comp.text() != "hello\n" || m.busy {
		t.Fatalf("ctrl+j: %q busy=%v", m.comp.text(), m.busy)
	}

	next, _ = m.Update(shiftEnterMsg{})
	m = next.(chatModel)
	if m.comp.text() != "hello\n\n" {
		t.Fatalf("shift+enter: %q", m.comp.text())
	}

	m.comp.set(`line\`)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(chatModel)
	if m.comp.text() != "line\n" || m.busy {
		t.Fatalf("backslash enter: %q busy=%v", m.comp.text(), m.busy)
	}

	m.comp.set(`keep\\`)
	if _, ok := continuedLine(m.comp.text()); ok {
		t.Fatal("escaped backslash should submit")
	}
	m.comp.set(`spaced\ `)
	if _, ok := continuedLine(m.comp.text()); ok {
		t.Fatal("a space after the slash should submit")
	}
}

func TestEnterCommitsTheUserLine(t *testing.T) {
	started := make(chan struct{})
	session := blockingSession(t, started)
	events := make(chan agent.Event, 64)
	session.Subscribe(func(ev agent.Event) { events <- ev })
	m := newChat(session, events)
	m.resize(80, 24)
	m.comp.insert("hello there")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(chatModel)
	if m.comp.text() != "" {
		t.Fatalf("composer not cleared: %q", m.comp.text())
	}
	if !strings.Contains(m.transcript(), "> hello there") {
		t.Fatal("user line was not committed")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "> hello there") || !strings.Contains(view, askPlaceholder) {
		t.Fatal("committed line is not in the transcript above a cleared composer")
	}
	for _, want := range []string{"Working", "esc to interrupt", "0s", askPlaceholder} {
		if !strings.Contains(view, want) {
			t.Fatalf("working row missing %q:\n%s", want, view)
		}
	}
	if lipglossHeight(m.View()) > 24 {
		t.Fatal("working frame is taller than the terminal")
	}

	done := make(chan chatModel, 1)
	go func() {
		done <- runCmds(t, m, cmd)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not start")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	select {
	case finished := <-done:
		if finished.busy {
			t.Fatal("esc left the turn running")
		}
		if finished.status != "interrupted" {
			t.Fatalf("status %q", finished.status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("esc did not interrupt")
	}
}

func TestElapsedClock(t *testing.T) {
	if fmtElapsed(0) != "0s" || fmtElapsed(3*time.Second) != "3s" {
		t.Fatal(fmtElapsed(3 * time.Second))
	}
	if fmtElapsed(65*time.Second) != "1m 05s" {
		t.Fatal(fmtElapsed(65 * time.Second))
	}
	if fmtElapsed(time.Hour+2*time.Minute+3*time.Second) != "1h 02m 03s" {
		t.Fatal(fmtElapsed(time.Hour))
	}
}

func blockingSession(t *testing.T, started chan struct{}) *coding.Session {
	t.Helper()
	session, err := coding.New(coding.Config{
		Workspace: t.TempDir(),
		APIKey:    "test",
		Model:     "test-model",
		Stream: func(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}
