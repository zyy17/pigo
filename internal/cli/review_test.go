package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
	"github.com/zyy17/pigo/internal/coding"
)

func TestEditReviewInTheChat(t *testing.T) {
	tests := []struct {
		name  string
		allow bool
		key   tea.KeyPressMsg
	}{
		{name: "allow", allow: true, key: tea.KeyPressMsg{Code: tea.KeyEnter}},
		{name: "deny", allow: false, key: tea.KeyPressMsg{Code: tea.KeyEsc}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "hello.txt")
			if err := os.WriteFile(path, []byte("alpha\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			session := reviewSession(t, dir)
			asked := make(chan confirmMsg, 1)
			session.SetConfirm(func(ctx context.Context, c coding.Confirm) (bool, error) {
				reply := make(chan bool, 1)
				asked <- confirmMsg{req: confirmRequest(c, reply)}
				select {
				case ok := <-reply:
					return ok, nil
				case <-ctx.Done():
					return false, ctx.Err()
				}
			})
			events := make(chan agent.Event, 64)
			session.Subscribe(func(ev agent.Event) { events <- ev })
			m := newChat(session)
			m.resize(80, 40)
			cmd := m.submit("edit hello.txt")

			done := make(chan struct{})
			go func() {
				execCmds(t, cmd)
				close(done)
			}()

			var msg confirmMsg
			select {
			case msg = <-asked:
			case <-time.After(3 * time.Second):
				t.Fatal("edit did not ask")
			}
			if got := mustRead(t, path); got != "alpha\n" {
				t.Fatalf("wrote before the answer: %q", got)
			}
			next, _ := m.Update(msg)
			m = next.(chatModel)
			view := ansi.Strip(m.View().Content)
			for _, want := range []string{"! allow this edit?", "-alpha", "+beta", "hello.txt", "enter", "esc"} {
				if !strings.Contains(view, want) {
					t.Errorf("view missing %q\n%s", want, view)
				}
			}
			if strings.Contains(view, "Working") {
				t.Fatalf("working row still shown:\n%s", view)
			}
			assertWidth(t, m.View().Content, 80)

			next, _ = m.Update(tt.key)
			m = next.(chatModel)
			if m.ask != nil {
				t.Fatal("confirm still open")
			}
			m = pumpReview(t, m, events, done)

			got := mustRead(t, path)
			plain := m.transcript()
			if tt.allow {
				if got != "beta\n" || !strings.Contains(plain, "updated hello.txt") {
					t.Fatalf("file %q\n%s", got, plain)
				}
				return
			}
			if got != "alpha\n" || !strings.Contains(plain, "blocked: file change denied") {
				t.Fatalf("file %q\n%s", got, plain)
			}
		})
	}
}

func TestConfirmDiffClips(t *testing.T) {
	m := readyChat(t)
	m.resize(80, 40)
	m.busy = true
	var body strings.Builder
	for i := range 10 {
		body.WriteString("+line ")
		body.WriteString(strings.Repeat("x", i))
		body.WriteByte('\n')
	}
	m.ask = &confirmReq{
		title:  "! allow this write?",
		body:   body.String(),
		detail: "notes/hello.txt",
		reply:  make(chan bool, 1),
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "+line") || !strings.Contains(view, "2 more lines") {
		t.Fatalf("clip\n%s", view)
	}
	if strings.Contains(view, "+line "+strings.Repeat("x", 9)) {
		t.Fatalf("last line was not clipped\n%s", view)
	}
	assertWidth(t, m.View().Content, 80)
}

func reviewSession(t *testing.T, dir string) *coding.Session {
	t.Helper()
	var step int
	session, err := coding.New(coding.Config{
		Workspace:     dir,
		APIKey:        "test",
		Model:         "test-model",
		UserSkillsDir: t.TempDir(),
		Stream: func(context.Context, ai.Request) (<-chan ai.Event, error) {
			step++
			n := step
			ch := make(chan ai.Event, 2)
			go func() {
				defer close(ch)
				if n == 1 {
					call := ai.ToolCall{
						ID:        "c1",
						Name:      "edit_file",
						Arguments: []byte(`{"path":"hello.txt","old_string":"alpha","new_string":"beta"}`),
					}
					ch <- ai.Event{Type: ai.EventDone, Message: ai.Assistant("", ai.StopToolUse, call)}
					return
				}
				ch <- ai.Event{Type: ai.EventTextDelta, Text: "done"}
				ch <- ai.Event{Type: ai.EventDone, Message: ai.Assistant("done", ai.StopStop)}
			}()
			return ch, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func pumpReview(t *testing.T, m chatModel, events <-chan agent.Event, done <-chan struct{}) chatModel {
	t.Helper()
	timeout := time.After(3 * time.Second)
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
		case <-timeout:
			t.Fatal("turn did not finish")
		}
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
