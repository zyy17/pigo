package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func TestLoopRunsToolThenStops(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse, call("call_1", "echo", map[string]any{"text": "hi"})),
		ai.Assistant("all done", ai.StopStop),
	}}
	ag := newAgent(t, script, echoTool())
	var labels []string
	ag.Subscribe(func(ev agent.Event) {
		labels = append(labels, label(ev))
	})

	if err := ag.Prompt(context.Background(), "say hi"); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"agent_start",
		"message_start:user",
		"message_end:user",
		"turn_start",
		"message_start:assistant",
		"message_end:assistant",
		"tool_execution_start",
		"tool_execution_end",
		"message_start:toolResult",
		"message_end:toolResult",
		"turn_end",
		"turn_start",
		"message_start:assistant",
		"message_update",
		"message_end:assistant",
		"turn_end",
		"agent_end",
	}, "\n")
	if strings.Join(labels, "\n") != want {
		t.Fatalf("events:\n%s", strings.Join(labels, "\n"))
	}

	if len(script.reqs) != 2 {
		t.Fatalf("model calls: %d", len(script.reqs))
	}
	if roles(script.reqs[0].Messages) != "system,user" {
		t.Fatalf("first request %s", roles(script.reqs[0].Messages))
	}
	if roles(script.reqs[1].Messages) != "system,user,assistant,toolResult" {
		t.Fatalf("second request %s", roles(script.reqs[1].Messages))
	}
	if script.reqs[0].Tools[0].Name != "echo" {
		t.Fatal("tool schema was not sent")
	}
	last := ag.Messages()
	if last[len(last)-1].Text != "all done" {
		t.Fatalf("transcript ended with %q", last[len(last)-1].Text)
	}
}

func TestInvalidToolJSON(t *testing.T) {
	var ran bool
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		ran = true
		return "ran", nil
	}
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse, ai.ToolCall{ID: "c1", Name: "echo", Arguments: []byte("{")}),
		ai.Assistant("recovered", ai.StopStop),
	}}
	ag := newAgent(t, script, tool)
	if err := ag.Prompt(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("execute ran on invalid JSON")
	}
	result := script.reqs[1].Messages[3]
	if result.Role != ai.RoleToolResult || !result.IsError || !strings.Contains(result.Text, "invalid") {
		t.Fatalf("tool result %#v", result)
	}
}

func TestUnknownToolAndToolError(t *testing.T) {
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		return "", errors.New("disk full")
	}
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse, call("c1", "missing", map[string]any{})),
		ai.Assistant("", ai.StopToolUse, call("c2", "echo", map[string]any{})),
		ai.Assistant("stopped", ai.StopStop),
	}}
	ag := newAgent(t, script, tool)
	if err := ag.Prompt(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	msgs := ag.Messages()
	if !msgs[2].IsError || !strings.Contains(msgs[2].Text, "unknown tool") {
		t.Fatalf("unknown tool result %#v", msgs[2])
	}
	if !msgs[4].IsError || !strings.Contains(msgs[4].Text, "disk full") {
		t.Fatalf("tool error %#v", msgs[4])
	}
}

func TestMaxTurns(t *testing.T) {
	callMsg := ai.Assistant("", ai.StopToolUse, call("c", "echo", map[string]any{}))
	script := &fakeModel{msgs: []ai.Message{callMsg, callMsg, callMsg}}
	ag, err := agent.New(agent.Config{
		Model:    "fake",
		MaxTurns: 2,
		Stream:   script.stream,
		Tools:    []agent.Tool{echoTool()},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = ag.Prompt(context.Background(), "loop")
	if err == nil || !strings.Contains(err.Error(), "2") {
		t.Fatal(err)
	}
}

func TestStreamErrorPairsToolCalls(t *testing.T) {
	script := &fakeModel{
		streamErr: errors.New("connection reset"),
		msgs: []ai.Message{
			ai.Assistant("partial", ai.StopToolUse, call("c1", "echo", map[string]any{"text": "x"})),
			ai.Assistant("recovered", ai.StopStop),
		},
	}
	var ran bool
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		ran = true
		return "ran", nil
	}
	ag := newAgent(t, script, tool)
	err := ag.Prompt(context.Background(), "go")
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("tool ran after a stream error")
	}
	if err := ag.Prompt(context.Background(), "again"); err != nil {
		t.Fatal(err)
	}
	if got := roles(script.reqs[1].Messages); got != "system,user,assistant,toolResult,user" {
		t.Fatalf("second request %s", got)
	}
	result := script.reqs[1].Messages[3]
	if !result.IsError || result.ToolCallID != "c1" || !strings.Contains(result.Text, "connection reset") {
		t.Fatalf("result %#v", result)
	}
}

func TestStopErrorPairsToolCalls(t *testing.T) {
	blocked := ai.Assistant("", ai.StopError, call("c1", "echo", map[string]any{}))
	blocked.Error = "content filter"
	script := &fakeModel{msgs: []ai.Message{
		blocked,
		ai.Assistant("next", ai.StopStop),
	}}
	var ran bool
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		ran = true
		return "ran", nil
	}
	ag := newAgent(t, script, tool)
	err := ag.Prompt(context.Background(), "go")
	if err == nil || !strings.Contains(err.Error(), "content filter") {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("tool ran after content filter")
	}
	if err := ag.Prompt(context.Background(), "again"); err != nil {
		t.Fatal(err)
	}
	if got := roles(script.reqs[1].Messages); got != "system,user,assistant,toolResult,user" {
		t.Fatalf("second request %s", got)
	}
}

func TestCloseCallsStartsTheTool(t *testing.T) {
	blocked := ai.Assistant("", ai.StopError, ai.ToolCall{
		ID:        "c1",
		Name:      "echo",
		Arguments: []byte(`{"text":"x"}`),
	})
	script := &fakeModel{msgs: []ai.Message{blocked}}
	ag := newAgent(t, script, echoTool())
	var sawStart bool
	var result string
	ag.Subscribe(func(ev agent.Event) {
		if ev.Type == agent.ToolStart && ev.ToolName == "echo" {
			sawStart = true
		}
		if ev.Type == agent.ToolEnd {
			result = ev.Result
		}
	})
	err := ag.Prompt(context.Background(), "go")
	if err == nil || !strings.Contains(err.Error(), "the model returned an error") {
		t.Fatal(err)
	}
	if !sawStart || result != "not executed" {
		t.Fatalf("start %v result %q", sawStart, result)
	}
}

func TestAbortPairsRemainingCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var ran int
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		ran++
		cancel()
		return "", context.Canceled
	}
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse,
			call("c1", "echo", map[string]any{}),
			call("c2", "echo", map[string]any{}),
		),
		ai.Assistant("should not run", ai.StopStop),
	}}
	ag := newAgent(t, script, tool)
	err := ag.Prompt(ctx, "stop halfway")
	if !errors.Is(err, agent.ErrInterrupted) {
		t.Fatal(err)
	}
	if ran != 1 {
		t.Fatalf("execute ran %d times", ran)
	}
	if len(script.reqs) != 1 {
		t.Fatalf("model calls after abort: %d", len(script.reqs))
	}
	var results int
	for _, msg := range ag.Messages() {
		if msg.Role == ai.RoleToolResult {
			results++
			if msg.Text != "aborted" || !msg.IsError {
				t.Fatalf("result %#v", msg)
			}
		}
	}
	if results != 2 {
		t.Fatalf("paired results: %d", results)
	}
}

func TestAbortStopsPromptChainedFromAgentEnd(t *testing.T) {
	var calls int
	var mu sync.Mutex
	stream := func(ctx context.Context, _ ai.Request) (<-chan ai.Event, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		ch := make(chan ai.Event, 1)
		if n == 1 {
			msg := ai.Assistant("done", ai.StopStop)
			ch <- ai.Event{Type: ai.EventDone, Message: msg}
			close(ch)
			return ch, nil
		}
		go func() {
			defer close(ch)
			<-ctx.Done()
			ch <- ai.Event{Type: ai.EventError, Err: ctx.Err()}
		}()
		return ch, nil
	}
	ag, err := agent.New(agent.Config{Model: "fake", Stream: stream})
	if err != nil {
		t.Fatal(err)
	}

	ready := make(chan struct{})
	secondErr := make(chan error, 1)
	var starts int
	ag.Subscribe(func(ev agent.Event) {
		if ev.Type == agent.AgentStart {
			starts++
			if starts == 2 {
				close(ready)
			}
			return
		}
		if ev.Type != agent.AgentEnd || starts != 1 {
			return
		}
		go func() {
			secondErr <- ag.Prompt(context.Background(), "queued")
		}()
		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("chained prompt did not start")
		}
	})

	if err := ag.Prompt(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	ag.Abort()
	select {
	case err := <-secondErr:
		if !errors.Is(err, agent.ErrInterrupted) {
			t.Fatalf("chained prompt: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("abort did not stop the chained prompt")
	}
}

func newAgent(t *testing.T, script *fakeModel, tool agent.Tool) *agent.Agent {
	t.Helper()
	ag, err := agent.New(agent.Config{
		SystemPrompt: "be helpful",
		Model:        "fake",
		Tools:        []agent.Tool{tool},
		Stream:       script.stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ag
}

func echoTool() agent.Tool {
	return agent.Tool{
		Name:        "echo",
		Description: "Echo the arguments",
		Parameters:  ai.ObjectSchema(map[string]ai.Property{"text": ai.StringProperty("text")}),
		Execute: func(ctx context.Context, args []byte) (string, error) {
			return "ok:" + string(args), nil
		},
	}
}

func call(id, name string, args any) ai.ToolCall {
	b, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return ai.ToolCall{ID: id, Name: name, Arguments: b}
}

func label(ev agent.Event) string {
	switch ev.Type {
	case agent.MessageStart, agent.MessageEnd:
		return string(ev.Type) + ":" + string(ev.Message.Role)
	default:
		return string(ev.Type)
	}
}

func roles(msgs []ai.Message) string {
	parts := make([]string, len(msgs))
	for i, msg := range msgs {
		parts[i] = string(msg.Role)
	}
	return strings.Join(parts, ",")
}

type fakeModel struct {
	mu        sync.Mutex
	msgs      []ai.Message
	i         int
	reqs      []ai.Request
	streamErr error
}

func (f *fakeModel) stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
	if f.i >= len(f.msgs) {
		return nil, fmt.Errorf("unexpected model call %d", f.i+1)
	}
	msg := f.msgs[f.i]
	f.i++
	streamErr := f.streamErr
	f.streamErr = nil
	ch := make(chan ai.Event, 2)
	go func() {
		defer close(ch)
		if streamErr != nil {
			ch <- ai.Event{Type: ai.EventError, Err: streamErr, Message: msg}
			return
		}
		if text := msg.Text; text != "" {
			ch <- ai.Event{Type: ai.EventTextDelta, Text: text}
		}
		ch <- ai.Event{Type: ai.EventDone, Message: msg}
	}()
	return ch, nil
}
