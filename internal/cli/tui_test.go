package cli

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	m := newChat(session)
	m.resize(80, 24)
	got := m.View().Content
	if h := lipgloss.Height(got); h > 24 {
		t.Fatalf("view is %d lines, taller than the terminal", h)
	}
	assertWidth(t, got, 80)
	opened := ansi.Strip(got)
	for _, want := range []string{"test-model", "enter send", "ctrl-c abort", "/exit", askPlaceholder, ">"} {
		if !strings.Contains(opened, want) {
			t.Fatalf("layout missing %q:\n%s", want, opened)
		}
	}
	for _, want := range []string{"pigo", "0.1.0", "██████"} {
		if !strings.Contains(opened, want) {
			t.Fatalf("transcript missing %q:\n%s", want, opened)
		}
	}

	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "# Title\nuse `code` and **bold**"})
	streaming := m.View().Content
	for _, want := range []string{"Title", "code", "bold"} {
		if !strings.Contains(streaming, want) {
			t.Fatalf("streaming view missing %q:\n%s", want, streaming)
		}
	}
	m.apply(agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("# Title\nuse `code` and **bold**", ai.StopStop),
	})
	done := ansi.Strip(m.View().Content)
	for _, want := range []string{"Title", "code", "bold", askPlaceholder} {
		if !strings.Contains(done, want) {
			t.Fatalf("finished reply missing %q:\n%s", want, done)
		}
	}
	if lipgloss.Height(m.View().Content) > 24 {
		t.Fatal("finished frame is taller than the terminal")
	}
}

func TestFooterFitsNarrowAndWide(t *testing.T) {
	short := composeFooter("ready", "glm-5.3", "/tmp/pigo-live", 80)
	if ansi.StringWidth(short) > 80 {
		t.Fatalf("short footer is %d cols: %s", ansi.StringWidth(short), short)
	}
	for _, want := range []string{"glm-5.3", "ctrl-c abort", "/exit"} {
		if !strings.Contains(short, want) {
			t.Fatalf("short footer missing %q: %s", want, short)
		}
	}

	long := composeFooter("working", strings.Repeat("m", 40), strings.Repeat("x", 80), 80)
	if ansi.StringWidth(long) > 80 {
		t.Fatalf("long footer is %d cols: %s", ansi.StringWidth(long), long)
	}
	if !strings.Contains(long, "…") {
		t.Fatalf("long footer was not truncated: %s", long)
	}

	m := newChat(nil)
	m.modelName = strings.Repeat("m", 40)
	m.root = strings.Repeat("x", 80)
	m.busy = true
	for _, width := range []int{80, 140} {
		m.resize(width, 24)
		got := m.View().Content
		if h := lipgloss.Height(got); h > 24 {
			t.Fatalf("width %d view is %d lines", width, h)
		}
		assertWidth(t, got, width)
	}
}

func TestLongTranscriptStaysInsideTheFrame(t *testing.T) {
	m := newChat(nil)
	m.resize(80, 24)
	var text strings.Builder
	for range 40 {
		text.WriteString("line\n")
	}
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: text.String()})
	if h := lipgloss.Height(m.View().Content); h > 24 {
		t.Fatalf("streaming view is %d lines", h)
	}
	assertWidth(t, m.View().Content, 80)
	if !strings.Contains(m.View().Content, "line") {
		t.Fatal("streaming reply is not in the frame")
	}
	m.apply(agent.Event{Type: agent.MessageEnd, Message: ai.Assistant(text.String(), ai.StopStop)})
	if h := lipgloss.Height(m.View().Content); h > 24 {
		t.Fatalf("after commit view is %d lines", h)
	}
	if !strings.Contains(m.transcript(), "line") || !strings.Contains(m.View().Content, "line") {
		t.Fatal("finished reply left the frame")
	}
	lines := strings.Split(m.View().Content, "\n")
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "ready") {
		t.Fatal("footer left the bottom")
	}
}

func TestChatShowsToolsAndText(t *testing.T) {
	m := newChat(nil)
	m.resize(80, 24)
	m.apply(agent.Event{Type: agent.ToolStart, ToolName: "write_file", Args: []byte(`{"path":"add.go"}`)})
	m.apply(agent.Event{Type: agent.ToolEnd, ToolName: "write_file", Result: "wrote add.go (20 bytes)"})
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "Added "})
	if !strings.Contains(m.View().Content, "Added") {
		t.Fatal("partial reply is not in the live view")
	}
	m.apply(agent.Event{Type: agent.MessageUpdate, TextDelta: "add.go."})
	m.apply(agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("Added add.go.", ai.StopStop),
	})
	m.apply(agent.Event{Type: agent.AgentEnd, Err: agent.ErrInterrupted})

	got := m.transcript()
	for _, want := range []string{"Ran write_file", "add.go", "└ wrote add.go", "Added add.go.", "interrupted"} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript missing %q:\n%s", want, got)
		}
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Added add.go.") || !strings.Contains(view, "interrupted") {
		t.Fatalf("finished reply left the frame:\n%s", view)
	}
}

func TestExitClearsTheLiveView(t *testing.T) {
	m := newChat(nil)
	m.resize(80, 24)
	if m.View().Content == "" {
		t.Fatal("expected a live view")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(chatModel)
	quit := m.View()
	if quit.Content != "" || quit.Cursor != nil || !quit.AltScreen {
		t.Fatal("quit left the input on screen")
	}
	if cmd == nil {
		t.Fatal("missing quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit command was %T", cmd())
	}
}

func TestCtrlDQuitsWhenIdle(t *testing.T) {
	m := newChat(nil)
	m.resize(40, 12)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = next.(chatModel)
	if !m.quitting || m.View().Content != "" {
		t.Fatal("ctrl-d did not quit")
	}
	if cmd == nil {
		t.Fatal("missing quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit command was %T", cmd())
	}

	m = newChat(nil)
	m.resize(40, 12)
	m.comp.insert("keep")
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = next.(chatModel)
	if m.quitting || cmd != nil {
		t.Fatal("ctrl-d quit with text in the composer")
	}

	m = newChat(nil)
	m.resize(40, 12)
	m.busy = true
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = next.(chatModel)
	if m.quitting || cmd != nil {
		t.Fatal("ctrl-d quit a running turn")
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
	m := newChat(session)
	m.resize(80, 24)
	m.comp.insert("/reset")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(chatModel)
	if !strings.Contains(m.transcript(), "transcript cleared") {
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
	m := newChat(session)
	m.resize(80, 24)
	model, cmd := m.submit("write hello.txt")
	m = model.(chatModel)
	m = runCmds(t, m, cmd, events)

	body, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil || string(body) != "hi\n" {
		t.Fatalf("file %q err %v", body, err)
	}
	got := m.transcript()
	for _, want := range []string{"> write hello.txt", "Ran write_file", "wrote hello.txt", "Created hello.txt."} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "> write hello.txt") != 1 {
		t.Fatalf("user line printed %d times:\n%s", strings.Count(got, "> write hello.txt"), got)
	}
	if m.busy {
		t.Fatal("chat still busy")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Created hello.txt.") || !strings.Contains(view, askPlaceholder) {
		t.Fatalf("finished frame:\n%s", view)
	}
}

func TestWrapKeepsWordsAndCaret(t *testing.T) {
	lines, line, col := wrapRunes([]rune("hello world"), 11, 8)
	if len(lines) != 2 || string(lines[0]) != "hello " || string(lines[1]) != "world" {
		t.Fatalf("word wrap: %#v", lines)
	}
	if line != 1 || col != 5 {
		t.Fatalf("caret at end: line %d col %d", line, col)
	}
	lines, line, col = wrapRunes([]rune("ab\ncd"), 3, 80)
	if string(lines[0]) != "ab" || string(lines[1]) != "cd" || line != 1 || col != 0 {
		t.Fatalf("newline caret: %#v line %d col %d", lines, line, col)
	}
	src := []rune(strings.Repeat("我们", 10))
	lines, _, _ = wrapRunes(src, len(src), 10)
	var all []rune
	for _, ln := range lines {
		if w := ansi.StringWidth(string(ln)); w > 10 {
			t.Fatalf("cjk line width %d: %q", w, string(ln))
		}
		all = append(all, ln...)
	}
	if string(all) != string(src) {
		t.Fatalf("lost runes: %q", string(all))
	}
}

func TestCaretStaysInTheInput(t *testing.T) {
	m := newChat(nil)
	m.modelName = "glm-5.3"
	m.root = "/tmp/pigo-live"
	m.resize(80, 24)
	m.comp.insert("我们额")
	got := m.View().Content
	assertWidth(t, got, 80)
	var inputLine, footer string
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "我们额") {
			inputLine = plain
		}
		if strings.Contains(plain, "ready") {
			footer = plain
		}
	}
	if inputLine == "" || !strings.HasPrefix(strings.TrimSpace(inputLine), ">") {
		t.Fatalf("CJK input missing from the composer: %q\n%s", inputLine, got)
	}
	if strings.Contains(footer, "我们") || !strings.Contains(footer, "ready") {
		t.Fatalf("footer overlapped the caret: %q", footer)
	}
}

func TestWrapCJKAndRewrapOnResize(t *testing.T) {
	src := strings.Repeat("我们", 30)
	got := wrapText(src, 10)
	if strings.ReplaceAll(got, "\n", "") != src {
		t.Fatalf("wrap dropped characters:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if w := ansi.StringWidth(line); w > 10 {
			t.Fatalf("line is %d cols: %q", w, line)
		}
	}
	if strings.Count(got, "\n") < 5 {
		t.Fatalf("expected several lines, got %q", got)
	}

	preview := toolPreview("a\nb\nc\nd\ne")
	if !strings.Contains(preview, "└ a") || !strings.Contains(preview, "+ 2 lines") {
		t.Fatalf("preview: %s", preview)
	}

	m := newChat(nil)
	m.resize(40, 24)
	m.addBlock(kindAssistant, src)
	wide := m.renderBlock(m.blocks[0])
	m.resize(16, 24)
	narrow := m.renderBlock(m.blocks[0])
	if strings.Count(narrow, "\n") <= strings.Count(wide, "\n") {
		t.Fatal("narrower width did not rewrap")
	}
	for _, line := range strings.Split(narrow, "\n") {
		if w := ansi.StringWidth(line); w > 16 {
			t.Fatalf("rewrapped line is %d cols: %q", w, line)
		}
	}
	m.live = src
	m.resize(20, 24)
	for _, line := range strings.Split(m.View().Content, "\n") {
		if w := ansi.StringWidth(line); w > 20 {
			t.Fatalf("resized live view is %d cols: %q", w, line)
		}
	}
}

func TestTranscriptScroll(t *testing.T) {
	m := newChat(nil)
	m.resize(80, 24)
	var text strings.Builder
	for i := range 80 {
		text.WriteString("row-")
		if i < 10 {
			text.WriteByte('0')
		}
		text.WriteString(strconv.Itoa(i))
		text.WriteByte('\n')
	}
	m.addBlock(kindAssistant, text.String())

	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "row-00") {
		t.Fatal("following frame still shows the oldest row")
	}
	if !strings.Contains(view, "row-79") {
		t.Fatalf("following frame missing the tail:\n%s", view)
	}
	lines := strings.Split(m.View().Content, "\n")
	if len(lines) != 24 {
		t.Fatalf("frame is %d lines", len(lines))
	}
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "ready") {
		t.Fatal("footer left the bottom")
	}

	room := m.transcriptRows()
	n := len(m.historyLines())
	start := m.viewTop(n, room)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = next.(chatModel)
	if moved := start - m.viewTop(n, room); moved != room-1 {
		t.Fatalf("page up moved %d lines, want %d", moved, room-1)
	}
	if m.scroll < 0 {
		t.Fatal("page up stayed pinned to the tail")
	}
	up := ansi.Strip(m.View().Content)
	if strings.Contains(up, "row-79") {
		t.Fatal("page up did not leave the tail")
	}
	if up == view {
		t.Fatal("page up did not change the frame")
	}
	paged := strings.Split(m.View().Content, "\n")
	if !strings.Contains(ansi.Strip(paged[len(paged)-1]), "ready") || !strings.Contains(ansi.Strip(m.View().Content), askPlaceholder) {
		t.Fatal("paging moved the composer or the footer")
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(chatModel)
	if m.scroll >= 0 || !strings.Contains(ansi.Strip(m.View().Content), "row-79") {
		t.Fatal("esc did not return to the tail")
	}

	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	m = next.(chatModel)
	if m.scroll < 0 || strings.Contains(ansi.Strip(m.View().Content), "row-79") {
		t.Fatal("wheel up did not scroll one row")
	}
	next, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = next.(chatModel)
	if m.scroll >= 0 || !strings.Contains(ansi.Strip(m.View().Content), "row-79") {
		t.Fatal("wheel down did not resume following")
	}

	m.scroll = 0
	held := ansi.Strip(m.View().Content)
	m.addBlock(kindInfo, "NEW-TAIL-LINE")
	if strings.Contains(ansi.Strip(m.View().Content), "NEW-TAIL-LINE") {
		t.Fatal("a scrolled frame followed new output")
	}
	if !strings.Contains(held, "row-00") && !strings.Contains(ansi.Strip(m.View().Content), "row-0") {
		t.Fatal("reading position moved")
	}
	m.scroll = followTail
	if !strings.Contains(ansi.Strip(m.View().Content), "NEW-TAIL-LINE") {
		t.Fatal("following frame missed new output")
	}

	m.scroll = 0
	m.busy = true
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if next.(chatModel).scroll < 0 {
		t.Fatal("esc while busy jumped to the tail")
	}

	m = newChat(nil)
	m.resize(80, 24)
	m.addBlock(kindAssistant, text.String())
	m.scroll = 0
	reply := make(chan bool, 1)
	m.ask = &confirmReq{command: "sudo true", reason: "sudo", reply: reply}
	m.busy = true
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(chatModel)
	if m.scroll < 0 || m.ask != nil {
		t.Fatal("esc during confirm jumped or left the prompt open")
	}
	if <-reply {
		t.Fatal("esc allowed the command")
	}
}

func assertWidth(t *testing.T, view string, width int) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Fatalf("line is %d cols, wider than %d: %q", w, width, line)
		}
	}
}

// runCmds runs the prompt command and applies session events the way
// program.Send would deliver them to Update.
func runCmds(t *testing.T, m chatModel, cmd tea.Cmd, events <-chan agent.Event) chatModel {
	t.Helper()
	done := make(chan struct{})
	go func() {
		execCmds(t, cmd)
		close(done)
	}()
	for {
		select {
		case ev := <-events:
			next, _ := m.Update(streamMsg{event: ev})
			m = next.(chatModel)
			if !m.busy {
				return m
			}
		case <-done:
			for {
				select {
				case ev := <-events:
					next, _ := m.Update(streamMsg{event: ev})
					m = next.(chatModel)
				default:
					return m
				}
			}
		}
	}
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
