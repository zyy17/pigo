package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/zyy17/pigo/internal/ai"
)

// Prompt runs one user task to completion.
//
// The body of the agent is run: ask the model, execute tool calls, ask
// again. Every tool call is paired with a tool-result message so the next
// request stays valid.
func (a *Agent) Prompt(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("prompt is empty")
	}

	ctx, cancel := context.WithCancel(ctx)
	a.setCancel(cancel)
	defer a.clearCancel()
	defer cancel()

	a.emit(Event{Type: AgentStart})
	user := ai.User(text)
	a.add(user)
	a.emit(Event{Type: MessageStart, Message: user})
	a.emit(Event{Type: MessageEnd, Message: user})

	err := a.run(ctx)
	if errors.Is(err, context.Canceled) {
		err = ErrInterrupted
	}
	a.emit(Event{Type: AgentEnd, Messages: a.Messages(), Err: err})
	return err
}

func (a *Agent) run(ctx context.Context) error {
	for turn := 1; turn <= a.maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.emit(Event{Type: TurnStart, Turn: turn})

		msg, err := a.readAssistant(ctx)
		if err != nil {
			a.emit(Event{Type: TurnEnd, Turn: turn, Message: msg, Err: err})
			return err
		}
		if !shouldRunTools(msg) {
			a.emit(Event{Type: TurnEnd, Turn: turn, Message: msg})
			return terminalError(msg)
		}
		if err := a.runTools(ctx, msg.ToolCalls); err != nil {
			a.emit(Event{Type: TurnEnd, Turn: turn, Message: msg, Err: err})
			return err
		}
		a.emit(Event{Type: TurnEnd, Turn: turn, Message: msg})
	}
	return fmt.Errorf("stopped after %d model turns", a.maxTurns)
}

func shouldRunTools(msg ai.Message) bool {
	if len(msg.ToolCalls) == 0 {
		return false
	}
	switch msg.StopReason {
	case ai.StopError, ai.StopAborted:
		return false
	default:
		return true
	}
}

func terminalError(msg ai.Message) error {
	if msg.StopReason != ai.StopError {
		return nil
	}
	if msg.Text != "" {
		return nil
	}
	if msg.Error != "" {
		return errors.New(msg.Error)
	}
	return errors.New("the model returned an error")
}

// complete reads one model stream, stores the assistant message, and
// forwards text deltas for the UI.
func (a *Agent) readAssistant(ctx context.Context) (ai.Message, error) {
	events, err := a.stream(ctx, a.request())
	if err != nil {
		return ai.Message{}, err
	}
	if events == nil {
		return ai.Message{}, errors.New("stream returned no events")
	}

	started := false
	for ev := range events {
		switch ev.Type {
		case ai.EventTextDelta:
			if !started {
				a.emit(Event{Type: MessageStart, Message: ai.Message{Role: ai.RoleAssistant}})
				started = true
			}
			a.emit(Event{Type: MessageUpdate, TextDelta: ev.Text, Message: ai.Message{Role: ai.RoleAssistant}})
		case ai.EventDone:
			msg := ev.Message
			if msg.Role == "" {
				msg.Role = ai.RoleAssistant
			}
			if !started {
				a.emit(Event{Type: MessageStart, Message: msg})
			}
			a.add(msg)
			a.emit(Event{Type: MessageEnd, Message: msg})
			return msg, nil
		case ai.EventError:
			return a.finishStreamError(started, ev)
		}
	}
	return ai.Message{}, errors.New("stream ended before the model finished")
}

func (a *Agent) finishStreamError(started bool, ev ai.Event) (ai.Message, error) {
	err := ev.Err
	if err == nil {
		err = errors.New("model stream failed")
	}
	msg := ev.Message
	if started || msg.Text != "" || len(msg.ToolCalls) > 0 {
		if msg.Role == "" {
			msg.Role = ai.RoleAssistant
		}
		msg.StopReason = ai.StopError
		msg.Error = err.Error()
		if !started {
			a.emit(Event{Type: MessageStart, Message: msg})
		}
		a.add(msg)
		a.emit(Event{Type: MessageEnd, Message: msg})
	}
	return msg, err
}

func (a *Agent) runTools(ctx context.Context, calls []ai.ToolCall) error {
	for i, call := range calls {
		if err := ctx.Err(); err != nil {
			a.abortRemaining(calls[i:])
			return err
		}
		a.runOneTool(ctx, call)
		if err := ctx.Err(); err != nil {
			a.abortRemaining(calls[i+1:])
			return err
		}
	}
	return nil
}

func (a *Agent) runOneTool(ctx context.Context, call ai.ToolCall) {
	args := normalizeArgs(call.Arguments)
	a.emit(Event{Type: ToolStart, ToolID: call.ID, ToolName: call.Name, Args: args})
	text, isErr := a.callTool(ctx, call, args)
	a.appendResult(call, text, isErr)
}

func (a *Agent) abortRemaining(calls []ai.ToolCall) {
	for _, call := range calls {
		args := normalizeArgs(call.Arguments)
		a.emit(Event{Type: ToolStart, ToolID: call.ID, ToolName: call.Name, Args: args})
		a.appendResult(call, "aborted", true)
	}
}

func (a *Agent) appendResult(call ai.ToolCall, text string, isError bool) {
	msg := ai.ToolResult(call.ID, call.Name, text, isError)
	a.add(msg)
	a.emit(Event{
		Type:     ToolEnd,
		ToolID:   call.ID,
		ToolName: call.Name,
		Result:   text,
		IsError:  isError,
	})
	a.emit(Event{Type: MessageStart, Message: msg})
	a.emit(Event{Type: MessageEnd, Message: msg})
}

func (a *Agent) callTool(ctx context.Context, call ai.ToolCall, args []byte) (string, bool) {
	if !json.Valid(args) {
		return "invalid tool arguments: " + clip(string(call.Arguments), 200), true
	}
	tool, ok := a.state.tool(call.Name)
	if !ok {
		return fmt.Sprintf("unknown tool %q", call.Name), true
	}
	if tool.Execute == nil {
		return fmt.Sprintf("tool %q has no execute function", call.Name), true
	}
	out, err := tool.Execute(ctx, args)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "aborted", true
		}
		return err.Error(), true
	}
	return out, false
}

func normalizeArgs(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
