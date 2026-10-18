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
	assertWidth(t, got, 80)
	for _, want := range []string{"test-model", "enter send", "ctrl-c abort", "/exit"} {
		if !strings.Contains(got, want) {
			t.Fatalf("layout missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "██████") {
		t.Fatal("wordmark is redrawn in the live view")
	}
	opened := strings.Join(m.printed, "\n")
	for _, want := range []string{"pigo", "0.1.0", "██████"} {
		if !strings.Contains(opened, want) {
			t.Fatalf("scrollback missing %q:\n%s", want, opened)
		}
	}

	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "# Title\nuse `code` and **bold**"})
	streaming := m.View()
	for _, want := range []string{"Title", "code", "bold"} {
		if !strings.Contains(streaming, want) {
			t.Fatalf("streaming view missing %q:\n%s", want, streaming)
		}
	}
	m.apply(agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("# Title\nuse `code` and **bold**", ai.StopStop),
	})
	if strings.Contains(m.View(), "Title") {
		t.Fatal("finished reply still in the live view")
	}
	done := strings.Join(m.printed, "\n")
	for _, want := range []string{"Title", "code", "bold"} {
		if !strings.Contains(done, want) {
			t.Fatalf("scrollback missing %q:\n%s", want, done)
		}
	}
}

func TestFooterFitsNarrowAndWide(t *testing.T) {
	m := newChat(nil, nil)
	m.modelName = strings.Repeat("m", 40)
	m.root = strings.Repeat("x", 80)
	m.busy = true
	for _, width := range []int{80, 120} {
		m.resize(width, 24)
		got := m.View()
		if h := lipgloss.Height(got); h > 24 {
			t.Fatalf("width %d view is %d lines", width, h)
		}
		assertWidth(t, got, width)
		if !strings.Contains(got, "ctrl-c abort") || !strings.Contains(got, "/exit") {
			t.Fatalf("width %d footer dropped hints:\n%s", width, got)
		}
	}
}

func TestFinishedTurnsLeaveTheLiveView(t *testing.T) {
	m := newChat(nil, nil)
	m.resize(80, 24)
	base := lipgloss.Height(m.View())
	var text strings.Builder
	for range 40 {
		text.WriteString("line\n")
	}
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: text.String()})
	if h := lipgloss.Height(m.View()); h > 24 {
		t.Fatalf("streaming view is %d lines", h)
	}
	assertWidth(t, m.View(), 80)
	m.apply(agent.Event{Type: agent.MessageEnd, Message: ai.Assistant(text.String(), ai.StopStop)})
	if lipgloss.Height(m.View()) != base {
		t.Fatalf("after commit view height %d want %d", lipgloss.Height(m.View()), base)
	}
	if !strings.Contains(strings.Join(m.printed, "\n"), "line") {
		t.Fatal("scrollback missing the finished reply")
	}
}

func TestChatShowsToolsAndText(t *testing.T) {
	m := newChat(nil, nil)
	m.resize(80, 24)
	m.apply(agent.Event{Type: agent.ToolStart, ToolName: "write_file", Args: []byte(`{"path":"add.go"}`)})
	m.apply(agent.Event{Type: agent.ToolEnd, ToolName: "write_file", Result: "wrote add.go (20 bytes)"})
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "Added "})
	if !strings.Contains(m.View(), "Added") {
		t.Fatal("partial reply is not in the live view")
	}
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "add.go."})
	m.apply(agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("Added add.go.", ai.StopStop),
	})
	m.apply(agent.Event{Type: agent.AgentEnd, Err: agent.ErrInterrupted})

	got := strings.Join(m.printed, "\n")
	for _, want := range []string{"→ write_file", "add.go", "← wrote add.go", "Added add.go.", "interrupted"} {
		if !strings.Contains(got, want) {
			t.Fatalf("scrollback missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(m.View(), "Added add.go.") {
		t.Fatal("finished reply still in the live view")
	}
}

func TestExitClearsTheLiveView(t *testing.T) {
	m := newChat(nil, nil)
	m.resize(80, 24)
	if m.View() == "" {
		t.Fatal("expected a live view")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(chatModel)
	if m.View() != "" {
		t.Fatal("quit left the input on screen")
	}
	if cmd == nil {
		t.Fatal("missing quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit command was %T", cmd())
	}
}

func TestResetPrintsSeparator(t *testing.T) {
	session, err := coding.New(coding.Config{
		Workspace: t.TempDir(),
		APIKey:    "test",
		Model:     "test-model",
		Stream: func(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
			return nil, context.Canceled
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	m := newChat(session, nil)
	m.resize(80, 24)
	m.input.SetValue("/reset")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(chatModel)
	if !strings.Contains(strings.Join(m.printed, "\n"), "transcript cleared") {
		t.Fatal("reset did not print a separator")
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
	got := strings.Join(m.printed, "\n")
	for _, want := range []string{"you> write hello.txt", "→ write_file", "← wrote hello.txt", "Created hello.txt."} {
		if !strings.Contains(got, want) {
			t.Fatalf("scrollback missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "you> write hello.txt") != 1 {
		t.Fatalf("user line printed %d times:\n%s", strings.Count(got, "you> write hello.txt"), got)
	}
	if m.busy {
		t.Fatal("chat still busy")
	}
	if strings.Contains(m.View(), "Created hello.txt.") {
		t.Fatal("finished reply still in the live view")
	}
}

func assertWidth(t *testing.T, view string, width int) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line is %d cols, wider than %d: %q", w, width, line)
		}
	}
}

// runCmds performs the prompt command and the event listener the way the
// Bubble Tea runtime would: both run together, then each event updates the model.
func runCmds(t *testing.T, m chatModel, cmd tea.Cmd) chatModel {
	t.Helper()
	pending := execCmds(t, cmd)
	for len(pending) > 0 && m.busy {
		msg := pending[0]
		pending = pending[1:]
		if _, ok := msg.(spinner.TickMsg); ok {
			next, _ := m.Update(msg)
			m = next.(chatModel)
			continue
		}
		next, follow := m.Update(msg)
		m = next.(chatModel)
		if m.busy {
			pending = append(pending, execCmds(t, follow)...)
		}
	}
	return m
}

func execCmds(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	results := make(chan tea.Msg, len(batch))
	var wg sync.WaitGroup
	for _, item := range batch {
		if item == nil {
			continue
		}
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
	var out []tea.Msg
	for got := range results {
		if nested, ok := got.(tea.BatchMsg); ok {
			out = append(out, execCmds(t, func() tea.Msg { return nested })...)
			continue
		}
		out = append(out, got)
	}
	return out
}
