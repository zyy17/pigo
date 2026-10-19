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
	started          time.Time
	cur              *frameCursor
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
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m *chatModel) resize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	if !m.ready {
		m.ready = true
		return m.printCell(m.opening())
	}
	// Finished lines are already in the scrollback at the width they were
	// printed. Reprinting them would duplicate the transcript. The composer,
	// the streaming reply, and the footer wrap again because they are redrawn.
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
	return m.onComposer(msg)
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
	return m.printCell(m.renderBlock(m.blocks[len(m.blocks)-1]))
}

// printCell writes one scrollback cell and the blank line after it.
// Every history cell goes through here, so the gap is the same each time.
func (m chatModel) printCell(cell string) tea.Cmd {
	cell = strings.TrimRight(cell, "\n")
	if cell == "" {
		return nil
	}
	return tea.Println(cell + "\n")
}
