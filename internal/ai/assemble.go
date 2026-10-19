package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type callBuf struct {
	id   string
	name string
	args strings.Builder
}

// accumulator folds streaming deltas into one assistant message.
type accumulator struct {
	text   strings.Builder
	calls  []callBuf
	index  map[int]int
	finish string
}

func (a *accumulator) consume(data string, out chan<- Event) error {
	var chunk wireChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return fmt.Errorf("bad stream chunk: %w", err)
	}
	if chunk.Error != nil && chunk.Error.Message != "" {
		return errors.New(chunk.Error.Message)
	}
	if len(chunk.Choices) == 0 {
		return nil
	}
	choice := chunk.Choices[0]
	if choice.Delta.Content != "" {
		a.text.WriteString(choice.Delta.Content)
		out <- Event{Type: EventTextDelta, Text: choice.Delta.Content}
	}
	for _, call := range choice.Delta.ToolCalls {
		a.absorb(call)
	}
	if choice.FinishReason != nil {
		a.finish = *choice.FinishReason
	}
	return nil
}

func (a *accumulator) absorb(call wireDeltaCall) {
	if a.index == nil {
		a.index = map[int]int{}
	}
	pos, ok := a.index[call.Index]
	if !ok {
		a.calls = append(a.calls, callBuf{})
		pos = len(a.calls) - 1
		a.index[call.Index] = pos
	}
	dst := &a.calls[pos]
	if call.ID != "" {
		dst.id = call.ID
	}
	if call.Function.Name != "" {
		dst.name = call.Function.Name
	}
	dst.args.WriteString(call.Function.Arguments)
}

func (a *accumulator) message() Message {
	calls := make([]ToolCall, 0, len(a.calls))
	for i, call := range a.calls {
		id := call.id
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		args := strings.TrimSpace(call.args.String())
		if args == "" {
			args = "{}"
		}
		calls = append(calls, ToolCall{
			ID:        id,
			Name:      call.name,
			Arguments: []byte(args),
		})
	}
	return Assistant(a.text.String(), finishReason(a.finish, len(calls)), calls...)
}

func finishReason(finish string, calls int) StopReason {
	switch finish {
	case "length":
		return StopLength
	case "content_filter":
		return StopError
	}
	if calls > 0 {
		return StopToolUse
	}
	if finish == "" || finish == "stop" {
		return StopStop
	}
	return StopError
}
