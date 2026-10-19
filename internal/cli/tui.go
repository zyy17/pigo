package cli

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
	"github.com/zyy17/pigo/internal/skills"
)

type chatModel struct {
	session          *coding.Session
	modelName        string
	root             string
	comp             composer
	spin             spinner.Model
	events           chan agent.Event
	blocks           []block
	skills           []skills.Skill
	live             string
	status           string
	width            int
	height           int
	suggestAt        int
	ready            bool
	busy             bool
	quitting         bool
	ask              *confirmReq
	suggestDismissed bool
	// follow pins the transcript to its latest line. top is the first
	// visible line while the user is reading older history.
	follow  bool
	top     int
	started time.Time
	cur     *frameCursor
}

type streamMsg struct {
	event agent.Event
}

// runTUI draws the chat on the alternate screen, the way Codex does.
// Quitting leaves that screen, which restores the shell buffer from
// before pigo started. Nothing is printed afterwards: there is no
// session id and no token total to report.
func runTUI(session *coding.Session) error {
	events := make(chan agent.Event, 256)
	session.Subscribe(func(ev agent.Event) {
		events <- ev
	})
	cur := &frameCursor{}
	model := newChat(session, events)
	model.cur = cur
	// Ask the terminal to distinguish Shift+Enter before the program reads keys,
	// and turn that mode off again so the user's shell is unchanged.
	enableShiftEnter(os.Stdout)
	defer disableShiftEnter(os.Stdout)
	// The closure captures program. Prompt cannot run until Run is looping,
	// so Send is never called on a nil program.
	var program *tea.Program
	if session != nil {
		session.SetConfirm(func(ctx context.Context, command, reason string) (bool, error) {
			reply := make(chan bool, 1)
			program.Send(confirmMsg{req: &confirmReq{command: command, reason: reason, reply: reply}})
			select {
			case ok := <-reply:
				return ok, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		})
	}
	program = tea.NewProgram(
		model,
		tea.WithOutput(&placingWriter{out: os.Stdout, file: os.Stdout, cur: cur}),
		tea.WithFilter(rewriteKeys),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := program.Run()
	return err
}

func newChat(session *coding.Session, events chan agent.Event) chatModel {
	m := chatModel{
		session: session,
		spin:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(statusWorkStyle)),
		events:  events,
		status:  "ready",
		follow:  true,
	}
	if session != nil {
		m.modelName = session.Model()
		m.root = session.Root()
		m.skills = session.Skills()
	}
	return m
}

func (m chatModel) Init() tea.Cmd { return nil }

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, m.resize(msg.Width, msg.Height)
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
	case shiftEnterMsg:
		if m.busy {
			return m, nil
		}
		return m.newline()
	case confirmMsg:
		m.ask = msg.req
		return m, nil
	case tea.MouseMsg:
		return m.onWheel(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m chatModel) onWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.scrollRows(-1), nil
	case tea.MouseButtonWheelDown:
		return m.scrollRows(1), nil
	default:
		return m, nil
	}
}

func (m *chatModel) resize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	m.ready = true
	// The transcript is wrapped again from its source on the next frame.
	// A reading position is a line offset, so a width change can drift it.
	if !m.follow {
		m.top = m.viewTop(len(m.historyLines()), m.transcriptRows())
	}
	return nil
}

func (m chatModel) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		if m.ask != nil {
			m = m.finishConfirm(false)
		}
		if m.busy {
			if m.session != nil {
				m.session.Abort()
			}
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit
	}
	// Ctrl-D quits when the composer is empty, matching Codex. A running
	// turn is left to Ctrl-C.
	if msg.Type == tea.KeyCtrlD && !m.busy && strings.TrimSpace(m.comp.text()) == "" {
		m.quitting = true
		return m, tea.Quit
	}
	if plainKey(msg) {
		switch msg.Type {
		case tea.KeyPgUp:
			return m.scrollRows(-m.pageRows()), nil
		case tea.KeyPgDown:
			return m.scrollRows(m.pageRows()), nil
		}
	}
	if m.ask != nil {
		return m.answerConfirm(msg)
	}
	if m.busy {
		// Esc stops the turn. The working row is the hint for that key.
		if msg.Type == tea.KeyEsc && m.session != nil {
			m.session.Abort()
		}
		return m, nil
	}
	// Esc returns a paused transcript to the tail. A slash menu owns Esc
	// first, and a running turn already used it to interrupt.
	if plainKey(msg) && msg.Type == tea.KeyEsc && !m.menuOpen() && m.canReturn() {
		m.follow = true
		m.top = 0
		return m, nil
	}
	return m.onComposer(msg)
}

func plainKey(msg tea.KeyMsg) bool {
	return !msg.Alt
}

func (m chatModel) submit(text string) (tea.Model, tea.Cmd) {
	switch strings.ToLower(text) {
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
		m.comp.clear()
		return m, m.addBlock(kindInfo, "— transcript cleared —")
	}
	expanded, err := commandText(m.session, text)
	if err != nil {
		m.comp.clear()
		return m, m.addBlock(kindError, err.Error())
	}
	m.busy = true
	m.started = time.Now()
	m.live = ""
	m.comp.clear()
	return m, tea.Batch(
		m.addBlock(kindUser, text),
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
		return nil
	case agent.MessageEnd:
		return m.finishMessage(ev.Message)
	case agent.ToolStart:
		return tea.Batch(m.flushLive(), m.addBlock(kindTool, ranTitle(ev.ToolName, ev.Args)))
	case agent.ToolEnd:
		preview := toolPreview(ev.Result)
		if preview == "" {
			return nil
		}
		return m.addBlock(kindTool, preview)
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
	text := m.takeLive()
	if strings.TrimSpace(text) == "" {
		text = msg.Text
	}
	return m.addBlock(kindAssistant, text)
}

func (m *chatModel) flushLive() tea.Cmd {
	return m.addBlock(kindAssistant, m.takeLive())
}

func (m *chatModel) takeLive() string {
	text := m.live
	m.live = ""
	return text
}

func (m *chatModel) noteEnd(err error) tea.Cmd {
	flush := m.flushLive()
	switch {
	case err == nil:
		m.status = "ready"
		return tea.Batch(flush, m.workedFor())
	case errors.Is(err, agent.ErrInterrupted):
		m.status = "interrupted"
		return tea.Batch(flush, m.addBlock(kindInfo, "interrupted"))
	default:
		m.status = "error"
		return tea.Batch(flush, m.addBlock(kindError, err.Error()))
	}
}

func (m *chatModel) workedFor() tea.Cmd {
	if m.started.IsZero() {
		return nil
	}
	secs := int(time.Since(m.started).Round(time.Second) / time.Second)
	m.started = time.Time{}
	if secs < 1 {
		return nil
	}
	return m.addBlock(kindInfo, "Worked for "+strconv.Itoa(secs)+"s")
}

func (m *chatModel) addBlock(kind blockKind, text string) tea.Cmd {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	m.blocks = append(m.blocks, block{kind: kind, text: text})
	return nil
}
