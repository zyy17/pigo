package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
)

type lineKind int

const (
	kindInfo lineKind = iota
	kindUser
	kindAssistant
	kindTool
	kindError
)

type chatLine struct {
	kind lineKind
	text string
}

type chatModel struct {
	session   *coding.Session
	input     textinput.Model
	view      viewport.Model
	events    chan agent.Event
	lines     []chatLine
	width     int
	height    int
	ready     bool
	busy      bool
	streaming bool
}

type streamMsg struct {
	event agent.Event
}

func runTUI(session *coding.Session) error {
	events := make(chan agent.Event, 256)
	session.Subscribe(func(ev agent.Event) {
		events <- ev
	})
	program := tea.NewProgram(newChat(session, events), tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func newChat(session *coding.Session, events chan agent.Event) chatModel {
	input := textinput.New()
	input.Prompt = "you> "
	input.Placeholder = "Write a function, edit a file, or ask a question"
	input.CharLimit = 8000
	input.Focus()
	return chatModel{
		session: session,
		input:   input,
		events:  events,
		lines: []chatLine{{
			kind: kindInfo,
			text: "Ask for a small coding task. /reset clears the chat. /exit quits.",
		}},
	}
}

func (m chatModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case streamMsg:
		m.apply(msg.event)
		if m.busy {
			return m, listen(m.events)
		}
		return m, nil
	case tea.KeyMsg:
		return m.onKey(msg)
	}

	if m.busy {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *chatModel) resize(width, height int) {
	m.width = width
	m.height = height
	body := height - 3
	if body < 1 {
		body = 1
	}
	if !m.ready {
		m.view = viewport.New(width, body)
		m.ready = true
	} else {
		m.view.Width = width
		m.view.Height = body
	}
	m.input.Width = width - len(m.input.Prompt)
	if m.input.Width < 1 {
		m.input.Width = 1
	}
	m.refresh()
}

func (m chatModel) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.busy {
			m.session.Abort()
			return m, nil
		}
		return m, tea.Quit
	case "pgup":
		if m.ready {
			m.view.HalfPageUp()
		}
		return m, nil
	case "pgdown":
		if m.ready {
			m.view.HalfPageDown()
		}
		return m, nil
	case "enter":
		if m.busy {
			return m, nil
		}
		return m.submit(strings.TrimSpace(m.input.Value()))
	}
	if m.busy {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m chatModel) submit(text string) (tea.Model, tea.Cmd) {
	switch text {
	case "":
		return m, nil
	case "/exit", "exit", "quit", "/quit":
		return m, tea.Quit
	case "/reset":
		m.session.Reset()
		m.add(kindInfo, "transcript cleared")
		m.input.SetValue("")
		return m, nil
	}
	m.busy = true
	m.streaming = false
	m.add(kindUser, text)
	m.input.SetValue("")
	return m, tea.Batch(listen(m.events), m.runPrompt(text))
}

func (m chatModel) runPrompt(text string) tea.Cmd {
	return func() tea.Msg {
		_ = m.session.Prompt(context.Background(), text)
		return nil
	}
}

func listen(events <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return nil
		}
		return streamMsg{event: ev}
	}
}

func (m *chatModel) apply(ev agent.Event) {
	switch ev.Type {
	case agent.MessageUpdate:
		m.appendDelta(ev.TextDelta)
	case agent.MessageEnd:
		m.finishMessage(ev.Message)
	case agent.ToolStart:
		m.streaming = false
		m.add(kindTool, "→ "+ev.ToolName+" "+flattenLine(string(ev.Args), 160))
	case agent.ToolEnd:
		mark := "←"
		if ev.IsError {
			mark = "✗"
		}
		m.add(kindTool, mark+" "+flattenLine(ev.Result, 240))
	case agent.AgentEnd:
		m.busy = false
		m.streaming = false
		m.noteEnd(ev.Err)
	}
}

func (m *chatModel) appendDelta(delta string) {
	if delta == "" {
		return
	}
	if m.streaming && len(m.lines) > 0 && m.lines[len(m.lines)-1].kind == kindAssistant {
		m.lines[len(m.lines)-1].text += delta
		m.refresh()
		return
	}
	m.streaming = true
	m.add(kindAssistant, delta)
}

func (m *chatModel) finishMessage(msg ai.Message) {
	if msg.Role == ai.RoleAssistant && msg.Text != "" && !m.streaming {
		m.add(kindAssistant, msg.Text)
	}
	m.streaming = false
}

func (m *chatModel) noteEnd(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, agent.ErrInterrupted) {
		m.add(kindInfo, "interrupted")
		return
	}
	m.add(kindError, err.Error())
}

func (m *chatModel) add(kind lineKind, text string) {
	m.lines = append(m.lines, chatLine{kind: kind, text: text})
	m.refresh()
}

func (m *chatModel) refresh() {
	if !m.ready {
		return
	}
	m.view.SetContent(renderTranscript(m.lines))
	m.view.GotoBottom()
}
