package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// streamChunk is one Server-Sent Event payload from chat completions.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

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
	done   bool
}

func (a *accumulator) consume(data string, out chan<- Event) error {
	var chunk streamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return fmt.Errorf("bad stream chunk: %w", err)
	}
	if chunk.Error != nil && chunk.Error.Message != "" {
		return fmt.Errorf("%s", chunk.Error.Message)
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
		a.absorb(call.Index, call.ID, call.Function.Name, call.Function.Arguments)
	}
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		a.finish = *choice.FinishReason
		a.done = true
	}
	return nil
}

func (a *accumulator) absorb(index int, id, name, args string) {
	if a.index == nil {
		a.index = map[int]int{}
	}
	pos, ok := a.index[index]
	if !ok {
		a.calls = append(a.calls, callBuf{})
		pos = len(a.calls) - 1
		a.index[index] = pos
	}
	call := &a.calls[pos]
	if id != "" {
		call.id = id
	}
	if name != "" {
		call.name = name
	}
	call.args.WriteString(args)
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
	return Assistant(a.text.String(), mapStop(a.finish, len(calls)), calls...)
}

func mapStop(finish string, calls int) StopReason {
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
