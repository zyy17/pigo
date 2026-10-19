package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func TestBeforeToolBlockKeepsTheTurnGoing(t *testing.T) {
	var ran int
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		ran++
		return "ok:" + string(args), nil
	}
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse,
			call("c1", "echo", map[string]any{"text": "no"}),
			call("c2", "echo", map[string]any{"text": "yes"}),
		),
		ai.Assistant("continued", ai.StopStop),
	}}
	ag := newAgentWithHook(t, script, tool, func(_ context.Context, call ai.ToolCall) (agent.Decision, error) {
		if call.ID == "c1" {
			return agent.Decision{Block: true, Reason: "nope"}, nil
		}
		return agent.Decision{}, nil
	})
	if err := ag.Prompt(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if ran != 1 {
		t.Fatalf("execute ran %d times", ran)
	}
	if len(script.reqs) != 2 {
		t.Fatalf("model calls: %d", len(script.reqs))
	}
	blocked := script.reqs[1].Messages[3]
	if blocked.Role != ai.RoleToolResult || !blocked.IsError || blocked.Text != "blocked: nope" {
		t.Fatalf("blocked result %#v", blocked)
	}
	ranCall := script.reqs[1].Messages[4]
	if ranCall.Role != ai.RoleToolResult || ranCall.IsError || !strings.Contains(ranCall.Text, "yes") {
		t.Fatalf("second result %#v", ranCall)
	}
	last := ag.Messages()
	if last[len(last)-1].Text != "continued" {
		t.Fatalf("transcript ended with %q", last[len(last)-1].Text)
	}
}

func TestBeforeToolRewriteReplacesArguments(t *testing.T) {
	var got []byte
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		got = append([]byte(nil), args...)
		return "ok", nil
	}
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse, call("c1", "echo", map[string]any{"text": "old"})),
		ai.Assistant("done", ai.StopStop),
	}}
	ag := newAgentWithHook(t, script, tool, func(context.Context, ai.ToolCall) (agent.Decision, error) {
		return agent.Decision{Args: []byte(`{"text":"rewritten"}`)}, nil
	})
	if err := ag.Prompt(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"text":"rewritten"}` {
		t.Fatalf("execute saw %s", got)
	}
}

func TestBeforeToolCancelStopsTheTurn(t *testing.T) {
	var ran int
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		ran++
		return "ok", nil
	}
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse,
			call("c1", "echo", map[string]any{"text": "a"}),
			call("c2", "echo", map[string]any{"text": "b"}),
		),
		ai.Assistant("should not run", ai.StopStop),
	}}
	ag := newAgentWithHook(t, script, tool, func(context.Context, ai.ToolCall) (agent.Decision, error) {
		return agent.Decision{}, context.Canceled
	})
	err := ag.Prompt(context.Background(), "go")
	if !errors.Is(err, agent.ErrInterrupted) {
		t.Fatalf("prompt err %v", err)
	}
	if ran != 0 {
		t.Fatalf("execute ran %d times", ran)
	}
	if len(script.reqs) != 1 {
		t.Fatalf("model calls: %d", len(script.reqs))
	}
	var results int
	for _, msg := range ag.Messages() {
		if msg.Role != ai.RoleToolResult {
			continue
		}
		results++
		if msg.Text != "aborted" || !msg.IsError {
			t.Fatalf("result %#v", msg)
		}
	}
	if results != 2 {
		t.Fatalf("paired results: %d", results)
	}
}

func newAgentWithHook(t *testing.T, script *fakeModel, tool agent.Tool, before agent.BeforeToolCall) *agent.Agent {
	t.Helper()
	ag, err := agent.New(agent.Config{
		SystemPrompt: "be helpful",
		Model:        "fake",
		Tools:        []agent.Tool{tool},
		Stream:       script.stream,
		Before:       before,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ag
}
