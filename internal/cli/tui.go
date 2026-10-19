// Chat model and turn lifecycle.
//
// runTUI is the only entry. Update routes terminal messages; apply folds
// agent events into the transcript. Keys live in keys.go, the frame in
// view.go, the composer in input.go, and scrolling in scroll.go.
package cli

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
	"github.com/zyy17/pigo/internal/skills"
)

type blockKind int

const (
	kindUser blockKind = iota
	kindAssistant
	kindRan
	kindTool
	kindWorked
	kindInfo
	kindError
)

type block struct {
	kind blockKind
	text string
}

type chatModel struct {
	session          *coding.Session
	modelName        string
	root             string
	comp             composer
	spin             spinner.Model
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
	// scroll is the first visible transcript line. Negative follows the tail.
	scroll  int
	started time.Time
}

type streamMsg struct {
	event agent.Event
}

// runTUI draws the chat on the alternate screen, the way Codex does.
// Quitting leaves that screen, which restores the shell buffer from
// before pigo started. Nothing is printed afterwards: there is no
// session id and no token total to report.
//
// The alternate screen, the mouse, and the cursor are fields on the
// view. Bubble Tea leaves the alternate screen when the program exits.
func runTUI(session *coding.Session) error {
	model := newChat(session)
	// Both closures capture program. Nothing calls Send until Run is looping.
	var program *tea.Program
	session.Subscribe(func(ev agent.Event) {
		program.Send(streamMsg{event: ev})
	})
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
	program = tea.NewProgram(model)
	_, err := program.Run()
	return err
}

func newChat(session *coding.Session) chatModel {
	m := chatModel{
		session: session,
		spin:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(statusWorkStyle)),
		status:  "ready",
		scroll:  followTail,
	}
	if session != nil {
		m.modelName = session.Model()
		m.root = session.Root()
		m.skills = session.Skills()
	}
	return m
}

// Init waits for the window size before the first frame.
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
		m.apply(msg.event)
		return m, nil
	case confirmMsg:
		m.ask = msg.req
		return m, nil
	case tea.PasteMsg:
		return m.onPaste(msg.Content)
	case tea.MouseWheelMsg:
		return m.onWheel(msg)
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m chatModel) onPaste(text string) (tea.Model, tea.Cmd) {
	if m.busy || m.ask != nil || text == "" {
		return m, nil
	}
	m.comp.insert(text)
	m.noteTyped()
	return m, nil
}

func (m *chatModel) resize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	m.ready = true
	// The transcript is wrapped again from its source on the next frame.
	// A reading position is a line offset, so a width change can drift it.
	if m.scroll >= 0 {
		m.scroll = m.viewTop(len(m.historyLines()), m.transcriptRows())
	}
	return nil
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
		m.addBlock(kindInfo, "— transcript cleared —")
		return m, nil
	}
	expanded, err := commandText(m.session, text)
	if err != nil {
		m.comp.clear()
		m.addBlock(kindError, err.Error())
		return m, nil
	}
	m.busy = true
	m.started = time.Now()
	m.live = ""
	m.comp.clear()
	m.addBlock(kindUser, text)
	return m, tea.Batch(m.runPrompt(expanded), m.spin.Tick)
}

func (m chatModel) runPrompt(text string) tea.Cmd {
	return func() tea.Msg {
		_ = m.session.Prompt(context.Background(), text)
		return nil
	}
}

func (m *chatModel) apply(ev agent.Event) {
	switch ev.Type {
	case agent.MessageUpdate:
		if ev.TextDelta == "" {
			return
		}
		m.live += ev.TextDelta
	case agent.MessageEnd:
		m.finishMessage(ev.Message)
	case agent.ToolStart:
		m.flushLive()
		m.addBlock(kindRan, ranTitle(ev.ToolName, ev.Args))
	case agent.ToolEnd:
		m.addBlock(kindTool, toolPreview(ev.Result))
	case agent.AgentEnd:
		m.busy = false
		m.noteEnd(ev.Err)
	}
}

func (m *chatModel) finishMessage(msg ai.Message) {
	if msg.Role != ai.RoleAssistant {
		return
	}
	text := m.takeLive()
	if strings.TrimSpace(text) == "" {
		text = msg.Text
	}
	m.addBlock(kindAssistant, text)
}

func (m *chatModel) flushLive() {
	m.addBlock(kindAssistant, m.takeLive())
}

func (m *chatModel) takeLive() string {
	text := m.live
	m.live = ""
	return text
}

func (m *chatModel) noteEnd(err error) {
	m.flushLive()
	switch {
	case err == nil:
		m.status = "ready"
		m.workedFor()
	case errors.Is(err, agent.ErrInterrupted):
		m.status = "interrupted"
		m.addBlock(kindInfo, "interrupted")
	default:
		m.status = "error"
		m.addBlock(kindError, err.Error())
	}
}

func (m *chatModel) workedFor() {
	if m.started.IsZero() {
		return
	}
	secs := int(time.Since(m.started).Round(time.Second) / time.Second)
	m.started = time.Time{}
	if secs < 1 {
		return
	}
	m.addBlock(kindWorked, "Worked for "+strconv.Itoa(secs)+"s")
}

func (m *chatModel) addBlock(kind blockKind, text string) {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	m.blocks = append(m.blocks, block{kind: kind, text: text})
}
