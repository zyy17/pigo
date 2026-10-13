package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func TestSessionEditsAFile(t *testing.T) {
	dir := t.TempDir()
	script := &scripted{msgs: []ai.Message{
		toolMsg("call_w", "write_file", map[string]any{"path": "hello.txt", "content": "alpha beta\n"}),
		toolMsg("call_e", "edit_file", map[string]any{
			"path": "hello.txt", "old_string": "beta", "new_string": "gamma",
		}),
		toolMsg("call_r", "read_file", map[string]any{"path": "hello.txt"}),
		ai.Assistant("the file now says gamma", ai.StopStop),
	}}
	session, err := New(Config{
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		Model:         "fake",
		Stream:        script.stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawRead bool
	session.Subscribe(func(ev agent.Event) {
		if ev.Type == agent.ToolEnd && ev.ToolName == "read_file" && strings.Contains(ev.Result, "gamma") {
			sawRead = true
		}
	})
	if err := session.Prompt(context.Background(), "update hello.txt"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(session.Root(), "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "alpha gamma\n" {
		t.Fatalf("file contents %q", body)
	}
	if !sawRead {
		t.Fatal("read_file did not observe the edit")
	}
	if len(script.requests()[0].Tools) != 5 {
		t.Fatalf("tool count %d", len(script.requests()[0].Tools))
	}

	session.Reset()
	script.append(ai.Assistant("fresh", ai.StopStop))
	if err := session.Prompt(context.Background(), "again"); err != nil {
		t.Fatal(err)
	}
	last := script.requests()
	req := last[len(last)-1]
	if got := roleList(req.Messages); got != "system,user" {
		t.Fatalf("after reset the model saw %s", got)
	}
	if req.Messages[1].Text != "again" {
		t.Fatalf("prompt %q", req.Messages[1].Text)
	}
}

func toolMsg(id, name string, args any) ai.Message {
	b, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return ai.Assistant("", ai.StopToolUse, ai.ToolCall{ID: id, Name: name, Arguments: b})
}

func roleList(msgs []ai.Message) string {
	parts := make([]string, len(msgs))
	for i, msg := range msgs {
		parts[i] = string(msg.Role)
	}
	return strings.Join(parts, ",")
}

type scripted struct {
	mu   sync.Mutex
	msgs []ai.Message
	i    int
	reqs []ai.Request
}

func (s *scripted) append(msg ai.Message) {
	s.mu.Lock()
	s.msgs = append(s.msgs, msg)
	s.mu.Unlock()
}

func (s *scripted) requests() []ai.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ai.Request, len(s.reqs))
	copy(out, s.reqs)
	return out
}

func (s *scripted) stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	if s.i >= len(s.msgs) {
		return nil, fmt.Errorf("unexpected model call %d", s.i+1)
	}
	msg := s.msgs[s.i]
	s.i++
	ch := make(chan ai.Event, 2)
	go func() {
		defer close(ch)
		if text := msg.Text; text != "" {
			ch <- ai.Event{Type: ai.EventTextDelta, Text: text}
		}
		ch <- ai.Event{Type: ai.EventDone, Message: msg}
	}()
	return ch, nil
}
