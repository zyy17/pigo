package cli

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
)

func TestCompactCommandParsesFocus(t *testing.T) {
	if got, ok := compactCommand("  /COMPACT  "); !ok || got != "" {
		t.Fatalf("%q %v", got, ok)
	}
	got, ok := compactCommand("/compact The Parser")
	if !ok || got != "The Parser" {
		t.Fatalf("%q %v", got, ok)
	}
	got, ok = compactCommand("/compact\nkeep the API")
	if !ok || got != "keep the API" {
		t.Fatalf("newline %q %v", got, ok)
	}
	if _, ok := compactCommand("/compactly"); ok {
		t.Fatal("prefix matched a longer word")
	}
	if sessionCommand("/compact please") != cmdCompact {
		t.Fatal("instructions were treated as a prompt")
	}
}

func TestCompactWaitsUntilTheTurnEnds(t *testing.T) {
	m := testChat(t)
	m.resize(40, 12)
	m.busy = true
	m.comp.set("/compact please")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if cmd != nil || m.queued != "" || !strings.Contains(m.transcript(), "/compact waits") {
		t.Fatalf("queued %q transcript %q", m.queued, m.transcript())
	}
}

func TestChatCompact(t *testing.T) {
	var focus string
	session := compactSession(t, func(req ai.Request) ai.Message {
		if len(req.Tools) == 0 {
			focus = req.Messages[1].Text
			return ai.Assistant("## Goal\nship", ai.StopStop)
		}
		return ai.Assistant("ok", ai.StopStop)
	})
	big := strings.Repeat("a", 40*4)
	if err := session.Prompt(context.Background(), big); err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), big); err != nil {
		t.Fatal(err)
	}
	events := make(chan agent.Event, 8)
	session.Subscribe(func(ev agent.Event) { events <- ev })

	m := newChat(session)
	m.resize(80, 24)
	m.comp.set("/compact The Parser")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if !m.busy || !strings.Contains(m.transcript(), "compacting…") {
		t.Fatalf("busy %v transcript %q", m.busy, m.transcript())
	}
	msgs := execCmds(t, cmd)
	for {
		select {
		case ev := <-events:
			next, _ = m.Update(streamMsg{event: ev})
			m = next.(chatModel)
		default:
			goto applied
		}
	}
applied:
	for _, msg := range msgs {
		next, _ = m.Update(msg)
		m = next.(chatModel)
	}
	if m.busy {
		t.Fatal("compact left the chat busy")
	}
	got := m.transcript()
	if !strings.Contains(got, "compacted older messages") || strings.Contains(got, "## Goal") {
		t.Fatalf("transcript:\n%s", got)
	}
	if !strings.Contains(focus, "Additional focus: The Parser") {
		t.Fatalf("focus %q", focus)
	}
}

func TestREPLCompact(t *testing.T) {
	session := compactSession(t, func(req ai.Request) ai.Message {
		if len(req.Tools) == 0 {
			return ai.Assistant("## Goal\nship", ai.StopStop)
		}
		return ai.Assistant("ok", ai.StopStop)
	})
	var out strings.Builder
	session.Subscribe(func(ev agent.Event) { render(&out, ev) })
	app := &app{session: session, in: strings.NewReader("/compact\n/exit\n"), out: &out}
	if err := app.repl(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "nothing to compact") {
		t.Fatalf("output:\n%s", out.String())
	}

	big := strings.Repeat("b", 40*4)
	if err := session.Prompt(context.Background(), big); err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), big); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	app.in = strings.NewReader("/compact keep the API\n/exit\n")
	if err := app.repl(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "compacted") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func compactSession(t *testing.T, reply func(ai.Request) ai.Message) *coding.Session {
	t.Helper()
	session, err := coding.New(coding.Config{
		Workspace:     t.TempDir(),
		UserSkillsDir: t.TempDir(),
		Model:         "fake",
		Compact: agent.CompactSettings{
			Enabled:          false,
			ContextWindow:    1000,
			ReserveTokens:    10,
			KeepRecentTokens: 8,
		},
		Stream: func(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
			msg := reply(req)
			ch := make(chan ai.Event, 2)
			go func() {
				defer close(ch)
				if msg.Text != "" {
					ch <- ai.Event{Type: ai.EventTextDelta, Text: msg.Text}
				}
				ch <- ai.Event{Type: ai.EventDone, Message: msg}
			}()
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}
