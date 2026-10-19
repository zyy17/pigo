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
	a.emit(Event{Type: AgentEnd, Err: err})
	return err
}

// run asks the model, runs any tool calls, and asks again.
// One pass is one model call. The next pass sees the tool results.
func (a *Agent) run(ctx context.Context) error {
	for turn := 1; turn <= a.maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.emit(Event{Type: TurnStart, Turn: turn})

		msg, err := a.readAssistant(ctx)
		if err != nil {
			a.closeCalls(msg)
			a.emit(Event{Type: TurnEnd, Turn: turn, Message: msg, Err: err})
			return err
		}
		if !shouldRunTools(msg) {
			a.closeCalls(msg)
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
	return len(msg.ToolCalls) > 0 && msg.StopReason != ai.StopError
}

// terminalError surfaces a model failure that produced no text.
// Text is the answer the user should see, so StopError with text still
// ends the turn cleanly.
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

// readAssistant reads one model stream and stores the assistant message.
// Deltas are for the UI. The done event is the message the transcript keeps.
func (a *Agent) readAssistant(ctx context.Context) (ai.Message, error) {
	events, err := a.stream(ctx, a.request())
	if err != nil {
		return ai.Message{}, err
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
		if err := a.runOneTool(ctx, call); err != nil {
			a.abortRemaining(calls[i+1:])
			return err
		}
	}
	// The last tool can cancel ctx and still return nil. The next turn
	// would otherwise start before Prompt notices.
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (a *Agent) runOneTool(ctx context.Context, call ai.ToolCall) error {
	call.Arguments = normalizeArgs(call.Arguments)
	if a.before != nil {
		d, err := a.before(ctx, call)
		if err != nil {
			return a.finishHook(call, err)
		}
		if d.Block {
			a.blockCall(call, d.Reason)
			return nil
		}
		if d.Args != nil {
			call.Arguments = d.Args
		}
	}
	a.emit(Event{Type: ToolStart, ToolID: call.ID, ToolName: call.Name, Args: call.Arguments})
	text, isErr := a.callTool(ctx, call)
	a.appendResult(call, text, isErr)
	return nil
}

// finishHook records the hook error as this call's result.
// A canceled hook stops the turn. Any other error stays on this call so the
// model can continue.
func (a *Agent) finishHook(call ai.ToolCall, err error) error {
	text := err.Error()
	if errors.Is(err, context.Canceled) {
		text = "aborted"
	}
	a.skipCall(call, text)
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return nil
}

func (a *Agent) blockCall(call ai.ToolCall, reason string) {
	if reason == "" {
		reason = "blocked"
	}
	a.skipCall(call, "blocked: "+reason)
}

// skipCall pairs a call that will not run with an error result.
func (a *Agent) skipCall(call ai.ToolCall, text string) {
	args := normalizeArgs(call.Arguments)
	a.emit(Event{Type: ToolStart, ToolID: call.ID, ToolName: call.Name, Args: args})
	a.appendResult(call, text, true)
}

// closeCalls records a result for tool calls that will not run.
// A stream error or a stop reason such as content_filter can leave calls on
// the assistant message. The next request is invalid without a result for each.
func (a *Agent) closeCalls(msg ai.Message) {
	if len(msg.ToolCalls) == 0 {
		return
	}
	text := msg.Error
	if text == "" {
		text = "not executed"
	}
	for _, call := range msg.ToolCalls {
		a.skipCall(call, text)
	}
}

// abortRemaining still records a result for each call that did not run.
// An assistant message with an unpaired tool call makes the next request invalid.
func (a *Agent) abortRemaining(calls []ai.ToolCall) {
	for _, call := range calls {
		a.skipCall(call, "aborted")
	}
}

// appendResult pairs a tool call with its result. The result uses the call's
// ID. The next model request needs that pair.
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

func (a *Agent) callTool(ctx context.Context, call ai.ToolCall) (string, bool) {
	if !json.Valid(call.Arguments) {
		return "invalid tool arguments: " + clip(string(call.Arguments), invalidArgRunes), true
	}
	tool, ok := a.state.tool(call.Name)
	if !ok {
		return fmt.Sprintf("unknown tool %q", call.Name), true
	}
	if tool.Execute == nil {
		return fmt.Sprintf("tool %q has no execute function", call.Name), true
	}
	out, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "aborted", true
		}
		return err.Error(), true
	}
	return out, false
}

// normalizeArgs fills an empty payload so a tool can decode JSON.
// The stream accumulator already writes "{}". This copy stays for a
// custom StreamFunc that leaves Arguments empty.
func normalizeArgs(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return []byte("{}")
	}
	return raw
}

const invalidArgRunes = 200

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
