package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
)

func TestChatLayout(t *testing.T) {
	dir := t.TempDir()
	session, err := coding.New(coding.Config{
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		APIKey:        "test",
		Model:         "test-model",
		Stream: func(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
			return nil, context.Canceled
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newChat(session, nil)
	m.resize(80, 24)
	got := m.View()
	if h := lipgloss.Height(got); h > 24 {
		t.Fatalf("view is %d lines, taller than the terminal", h)
	}
	for _, want := range []string{"pigo", "0.1.0", "test-model", "enter send", "ctrl-c abort", "/exit"} {
		if !strings.Contains(got, want) {
			t.Fatalf("layout missing %q:\n%s", want, got)
		}
	}
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "# Title\nuse `code` and **bold**"})
	text := renderTranscript(m.lines)
	for _, want := range []string{"Title", "code", "bold"} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown missing %q:\n%s", want, text)
		}
	}
}

func TestChatShowsToolsAndText(t *testing.T) {
	m := newChat(nil, nil)
	m.apply(agent.Event{Type: agent.ToolStart, ToolName: "write_file", Args: []byte(`{"path":"add.go"}`)})
	m.apply(agent.Event{Type: agent.ToolEnd, ToolName: "write_file", Result: "wrote add.go (20 bytes)"})
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "Added "})
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "add.go."})
	m.apply(agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("Added add.go.", ai.StopStop),
	})
	m.apply(agent.Event{Type: agent.AgentEnd, Err: agent.ErrInterrupted})

	got := renderTranscript(m.lines)
	for _, want := range []string{"→ write_file", "add.go", "← wrote add.go", "Added add.go.", "interrupted"} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript missing %q:\n%s", want, got)
		}
	}
}

func TestChatRunsAWrite(t *testing.T) {
	dir := t.TempDir()
	var step int
	session, err := coding.New(coding.Config{
		Workspace: dir,
		APIKey:    "test",
		Stream: func(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
			step++
			n := step
			ch := make(chan ai.Event, 4)
			go func() {
				defer close(ch)
				if n == 1 {
					call := ai.ToolCall{
						ID:        "c1",
						Name:      "write_file",
						Arguments: []byte(`{"path":"hello.txt","content":"hi\n"}`),
					}
					ch <- ai.Event{Type: ai.EventDone, Message: ai.Assistant("", ai.StopToolUse, call)}
					return
				}
				ch <- ai.Event{Type: ai.EventTextDelta, Text: "Created hello.txt."}
				ch <- ai.Event{Type: ai.EventDone, Message: ai.Assistant("Created hello.txt.", ai.StopStop)}
			}()
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan agent.Event, 64)
	session.Subscribe(func(ev agent.Event) { events <- ev })
	m := newChat(session, events)
	m.resize(80, 24)
	model, cmd := m.submit("write hello.txt")
	m = model.(chatModel)
	m = runCmds(t, m, cmd)

	body, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil || string(body) != "hi\n" {
		t.Fatalf("file %q err %v", body, err)
	}
	got := renderTranscript(m.lines)
	for _, want := range []string{"you> write hello.txt", "→ write_file", "← wrote hello.txt", "Created hello.txt."} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript missing %q:\n%s", want, got)
		}
	}
	if m.busy {
		t.Fatal("chat still busy")
	}
}

// runCmds performs the prompt command and the event listener the way the
// Bubble Tea runtime would: both run together, then each event updates the model.
func runCmds(t *testing.T, m chatModel, cmd tea.Cmd) chatModel {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("command was %T", msg)
	}
	results := make(chan tea.Msg, len(batch))
	var wg sync.WaitGroup
	for _, item := range batch {
		wg.Add(1)
		go func(item tea.Cmd) {
			defer wg.Done()
			if got := item(); got != nil {
				results <- got
			}
		}(item)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	for got := range results {
		next, follow := m.Update(got)
		m = next.(chatModel)
		for follow != nil && m.busy {
			// A spinner tick waits on a timer and would keep this loop alive.
			if _, ok := got.(spinner.TickMsg); ok {
				break
			}
			got = follow()
			if got == nil {
				break
			}
			next, follow = m.Update(got)
			m = next.(chatModel)
		}
	}
	return m
}
