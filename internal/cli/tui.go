package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
)

const inputLines = 3

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
	modelName string
	root      string
	input     textarea.Model
	spin      spinner.Model
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
	input := textarea.New()
	input.Placeholder = "Ask for a coding task"
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.CharLimit = 0
	input.SetHeight(inputLines)
	// Enter sends the box. A newline is an explicit chord so a task can span lines.
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	input.Focus()

	m := chatModel{
		session: session,
		input:   input,
		spin:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(cyanStyle)),
		events:  events,
		lines: []chatLine{{
			kind: kindInfo,
			text: "/skill:name loads a skill. /reset clears the chat. /exit quits.",
		}},
	}
	if session != nil {
		m.modelName = session.Model()
		m.root = session.Root()
	}
	return m
}

func (m chatModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
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
	frame := inputStyle.Width(width)
	inner := width - frame.GetHorizontalFrameSize()
	if inner < 1 {
		inner = 1
	}
	m.input.SetWidth(inner)
	m.input.SetHeight(inputLines)
	body := height - lipgloss.Height(m.header()) - lipgloss.Height(m.footer()) - inputLines - frame.GetVerticalFrameSize()
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
	case "enter", "ctrl+m":
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
	expanded, err := commandText(m.session, text)
	if err != nil {
		m.add(kindError, err.Error())
		m.input.SetValue("")
		return m, nil
	}
	m.busy = true
	m.streaming = false
	m.add(kindUser, text)
	m.input.SetValue("")
	return m, tea.Batch(listen(m.events), m.runPrompt(expanded), m.spin.Tick)
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
