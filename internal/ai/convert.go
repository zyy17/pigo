package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// wireMessage is one OpenAI chat message. Tool call fields are flat here.
// MarshalJSON on wireCall adds the nested "function" object the API expects.
type wireMessage struct {
	Role       string     `json:"role"`
	Content    *string    `json:"content,omitempty"`
	ToolCalls  []wireCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// wireCall is one tool call. Name and Arguments are not nested in Go.
type wireCall struct {
	ID        string
	Name      string
	Arguments string
}

type wireFunctionBody struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireCallBody struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionBody `json:"function"`
}

func (c wireCall) MarshalJSON() ([]byte, error) {
	return marshalValue(wireCallBody{
		ID:   c.ID,
		Type: "function",
		Function: wireFunctionBody{
			Name:      c.Name,
			Arguments: c.Arguments,
		},
	})
}

// wireTool is one tool schema. MarshalJSON wraps it as {"type":"function",...}.
type wireTool struct {
	Name        string
	Description string
	Parameters  Schema
}

type wireToolBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  Schema `json:"parameters"`
}

type wireToolEnvelope struct {
	Type     string       `json:"type"`
	Function wireToolBody `json:"function"`
}

func (t wireTool) MarshalJSON() ([]byte, error) {
	return marshalValue(wireToolEnvelope{
		Type: "function",
		Function: wireToolBody{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		},
	})
}

func marshalValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

type wireRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
}

func encodeRequest(req Request) ([]byte, error) {
	messages, err := toWireMessages(req.Messages)
	if err != nil {
		return nil, err
	}
	body := wireRequest{
		Model:    req.Model,
		Messages: messages,
		Tools:    toWireTools(req.Tools),
		Stream:   true,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func toWireMessages(msgs []Message) ([]wireMessage, error) {
	out := make([]wireMessage, 0, len(msgs))
	for _, msg := range msgs {
		wire, err := toWireMessage(msg)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return out, nil
}

// toWireMessage maps a Pi-style transcript message onto the chat API.
// Assistant tool calls stay on the assistant message. Tool results, which
// are their own role in the transcript, become role "tool".
func toWireMessage(msg Message) (wireMessage, error) {
	switch msg.Role {
	case RoleSystem, RoleUser:
		text := msg.Text
		return wireMessage{Role: string(msg.Role), Content: &text}, nil
	case RoleAssistant:
		return wireFromAssistant(msg), nil
	case RoleToolResult:
		text := msg.Text
		if msg.IsError {
			text = "error: " + text
		}
		return wireMessage{Role: "tool", Content: &text, ToolCallID: msg.ToolCallID}, nil
	default:
		return wireMessage{}, fmt.Errorf("unknown message role %q", msg.Role)
	}
}

func wireFromAssistant(msg Message) wireMessage {
	wire := wireMessage{Role: "assistant"}
	if msg.Text != "" {
		text := msg.Text
		wire.Content = &text
	}
	for _, call := range msg.ToolCalls {
		args := strings.TrimSpace(string(call.Arguments))
		if args == "" {
			args = "{}"
		}
		wire.ToolCalls = append(wire.ToolCalls, wireCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: args,
		})
	}
	return wire
}

func toWireTools(tools []Tool) []wireTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, wireTool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}
	return out
}
