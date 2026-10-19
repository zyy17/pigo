package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// These types mirror the OpenAI chat-completions JSON. Each one is a single
// object. Nesting is a field that names the next type, so the shape can be
// read from top to bottom.

type wireRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireToolSpec `json:"function"`
}

type wireToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  Schema `json:"parameters"`
}

type wireChunk struct {
	Choices []wireChoice `json:"choices"`
	Error   *wireError   `json:"error,omitempty"`
}

type wireError struct {
	Message string `json:"message"`
}

type wireChoice struct {
	Delta        wireDelta `json:"delta"`
	FinishReason *string   `json:"finish_reason"`
}

type wireDelta struct {
	Content   string          `json:"content,omitempty"`
	ToolCalls []wireDeltaCall `json:"tool_calls,omitempty"`
}

type wireDeltaCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Function wireFunction `json:"function,omitempty"`
}

// encodeRequest translates the transcript into a chat-completions body.
// Tool results stay role toolResult in memory and become role "tool" only here.
func encodeRequest(req Request) ([]byte, error) {
	messages, err := wireMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireRequest{
		Model:    req.Model,
		Messages: messages,
		Tools:    wireTools(req.Tools),
		Stream:   true,
	})
}

func wireMessages(msgs []Message) ([]wireMessage, error) {
	out := make([]wireMessage, 0, len(msgs))
	for _, msg := range msgs {
		wire, err := wireMessageOf(msg)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return out, nil
}

func wireMessageOf(msg Message) (wireMessage, error) {
	switch msg.Role {
	case RoleSystem, RoleUser:
		return wireMessage{Role: string(msg.Role), Content: strPtr(msg.Text)}, nil
	case RoleAssistant:
		return assistantWire(msg), nil
	case RoleToolResult:
		text := msg.Text
		if msg.IsError {
			text = "error: " + text
		}
		return wireMessage{Role: "tool", Content: strPtr(text), ToolCallID: msg.ToolCallID}, nil
	default:
		return wireMessage{}, fmt.Errorf("unknown message role %q", msg.Role)
	}
}

func assistantWire(msg Message) wireMessage {
	wire := wireMessage{Role: "assistant"}
	if msg.Text != "" {
		wire.Content = strPtr(msg.Text)
	}
	for _, call := range msg.ToolCalls {
		args := strings.TrimSpace(string(call.Arguments))
		if args == "" {
			args = "{}"
		}
		wire.ToolCalls = append(wire.ToolCalls, wireToolCall{
			ID:   call.ID,
			Type: "function",
			Function: wireFunction{
				Name:      call.Name,
				Arguments: args,
			},
		})
	}
	return wire
}

func wireTools(tools []Tool) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, wireTool{
			Type:     "function",
			Function: wireToolSpec(tool),
		})
	}
	return out
}

func strPtr(s string) *string { return &s }

// streamDelta is the piece of one SSE payload the accumulator stores.
type streamDelta struct {
	text    string
	calls   []wireDeltaCall
	finish  string
	errText string
}

func decodeChunk(data string) (streamDelta, error) {
	var chunk wireChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return streamDelta{}, fmt.Errorf("bad stream chunk: %w", err)
	}
	out := streamDelta{}
	if chunk.Error != nil {
		out.errText = chunk.Error.Message
	}
	if len(chunk.Choices) == 0 {
		return out, nil
	}
	choice := chunk.Choices[0]
	if choice.FinishReason != nil {
		out.finish = *choice.FinishReason
	}
	out.text = choice.Delta.Content
	out.calls = choice.Delta.ToolCalls
	return out, nil
}
