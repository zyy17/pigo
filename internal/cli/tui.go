package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
)

const inputLines = 3

type chatModel struct {
	session   *coding.Session
	modelName string
	root      string
	input     textarea.Model
	spin      spinner.Model
	events    chan agent.Event
	printed   []string
	live      string
	width     int
	height    int
	ready     bool
	busy      bool
	quitting  bool
}

type streamMsg struct {
	event agent.Event
}

// runTUI uses the normal screen. Finished lines go to the terminal scrollback.
// View redraws only the streaming reply, the input, and the footer.
func runTUI(session *coding.Session) error {
	events := make(chan agent.Event, 256)
	session.Subscribe(func(ev agent.Event) {
		events <- ev
	})
	program := tea.NewProgram(newChat(session, events))
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
		cmd := m.resize(msg.Width, msg.Height)
		return m, cmd
	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case streamMsg:
		cmd := m.apply(msg.event)
		if m.busy {
			return m, tea.Batch(listen(m.events), cmd)
		}
		return m, cmd
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

func (m *chatModel) resize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	m.fitInput()
	if m.ready {
		return nil
	}
	m.ready = true
	return m.commitRaw(m.opening())
}

func (m *chatModel) fitInput() {
	pad := inputStyle.GetHorizontalPadding()
	inner := m.boxWidth() - pad
	if inner < 1 {
		inner = 1
	}
	m.input.SetWidth(inner)
	m.input.SetHeight(m.inputLineCount())
}

// boxWidth is the input style width. The border sits outside that width,
// so the style itself is narrower than the terminal by the border.
func (m chatModel) boxWidth() int {
	width := m.width
	if width < 1 {
		width = 1
	}
	inner := width - inputStyle.GetHorizontalBorderSize()
	if inner < 1 {
		return 1
	}
	return inner
}

func (m chatModel) inputLineCount() int {
	const border = 2
	const foot = 2
	room := m.height - border - foot
	if m.live != "" && room > 1 {
		room--
	}
	if room < 1 {
		room = 1
	}
	if room > inputLines {
		return inputLines
	}
	return room
}

func (m chatModel) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.busy {
			if m.session != nil {
				m.session.Abort()
			}
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
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
		m.quitting = true
		return m, tea.Quit
	case "/reset":
		if m.session != nil {
			m.session.Reset()
		}
		m.live = ""
		m.fitInput()
		m.input.SetValue("")
		return m, m.commitPlain("— transcript cleared —", dimStyle)
	}
	expanded, err := commandText(m.session, text)
	if err != nil {
		m.input.SetValue("")
		return m, m.commitPlain(err.Error(), errStyle)
	}
	m.busy = true
	m.live = ""
	m.fitInput()
	m.input.SetValue("")
	return m, tea.Batch(
		m.commitPlain("you> "+text, userStyle),
		listen(m.events),
		m.runPrompt(expanded),
		m.spin.Tick,
	)
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

func (m *chatModel) apply(ev agent.Event) tea.Cmd {
	switch ev.Type {
	case agent.MessageUpdate:
		if ev.TextDelta == "" {
			return nil
		}
		m.live += ev.TextDelta
		m.fitInput()
		return nil
	case agent.MessageEnd:
		return m.finishMessage(ev.Message)
	case agent.ToolStart:
		return tea.Batch(m.flushLive(), m.commitPlain(toolStartText(ev), toolStyle))
	case agent.ToolEnd:
		return m.commitPlain(toolEndText(ev), toolStyle)
	case agent.AgentEnd:
		m.busy = false
		return m.noteEnd(ev.Err)
	default:
		return nil
	}
}

func (m *chatModel) finishMessage(msg ai.Message) tea.Cmd {
	if msg.Role != ai.RoleAssistant {
		return nil
	}
	text := m.live
	m.live = ""
	m.fitInput()
	if strings.TrimSpace(text) == "" {
		text = msg.Text
	}
	return m.commitAssistant(text)
}

func (m *chatModel) flushLive() tea.Cmd {
	text := m.live
	m.live = ""
	m.fitInput()
	return m.commitAssistant(text)
}

func (m *chatModel) noteEnd(err error) tea.Cmd {
	flush := m.flushLive()
	if err == nil {
		return flush
	}
	if errors.Is(err, agent.ErrInterrupted) {
		return tea.Batch(flush, m.commitPlain("interrupted", dimStyle))
	}
	return tea.Batch(flush, m.commitPlain(err.Error(), errStyle))
}

func toolStartText(ev agent.Event) string {
	return "→ " + ev.ToolName + " " + flattenLine(string(ev.Args), 160)
}

func toolEndText(ev agent.Event) string {
	mark := "←"
	if ev.IsError {
		mark = "✗"
	}
	return mark + " " + flattenLine(ev.Result, 240)
}

// commitRaw prints a finished block into the terminal scrollback.
func (m *chatModel) commitRaw(block string) tea.Cmd {
	if strings.TrimSpace(block) == "" {
		return nil
	}
	block = strings.TrimRight(block, "\n")
	m.printed = append(m.printed, block)
	return tea.Println(block)
}

func (m *chatModel) commitPlain(text string, style lipgloss.Style) tea.Cmd {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	if m.width > 0 {
		text = wrapPlain(text, m.width)
	}
	return m.commitRaw(style.Render(text))
}

func (m *chatModel) commitAssistant(text string) tea.Cmd {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if m.width > 0 {
		text = wrapPlain(text, m.width)
	}
	return m.commitRaw(renderAssistant(text))
}
