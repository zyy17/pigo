package ai

import "strings"

// Role is who produced a transcript message.
// Tool results stay their own role until the HTTP client maps them to
// OpenAI's "tool" role.
type Role string

const (
	RoleSystem     Role = "system"
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "toolResult"
)

// StopReason says why an assistant message ended.
type StopReason string

const (
	StopStop    StopReason = "stop"
	StopToolUse StopReason = "toolUse"
	StopLength  StopReason = "length"
	StopError   StopReason = "error"
)

// ToolCall is one function call the model asked for.
// Arguments is the raw JSON object, still unparsed.
type ToolCall struct {
	ID        string
	Name      string
	Arguments []byte
}

// Message is one transcript entry.
// Text and ToolCalls sit directly on the message so the shape stays flat.
// ToolCallID, ToolName, and IsError are set only on tool results.
type Message struct {
	Role       Role
	Text       string
	ToolCalls  []ToolCall
	ToolCallID string
	ToolName   string
	IsError    bool
	StopReason StopReason
	Error      string
}

// System builds a system message.
func System(text string) Message {
	return Message{Role: RoleSystem, Text: text}
}

// User builds a user message.
func User(text string) Message {
	return Message{Role: RoleUser, Text: text}
}

// Assistant builds an assistant message from text and tool calls.
func Assistant(text string, reason StopReason, calls ...ToolCall) Message {
	return Message{Role: RoleAssistant, Text: text, ToolCalls: calls, StopReason: reason}
}

// ToolResult builds the message that answers one tool call.
func ToolResult(id, name, text string, isError bool) Message {
	return Message{
		Role:       RoleToolResult,
		Text:       text,
		ToolCallID: id,
		ToolName:   name,
		IsError:    isError,
	}
}

// WithSystem prepends a system message when prompt is non-empty.
// The returned slice is new. msgs is left unchanged.
func WithSystem(prompt string, msgs []Message) []Message {
	if strings.TrimSpace(prompt) == "" {
		out := make([]Message, len(msgs))
		copy(out, msgs)
		return out
	}
	out := make([]Message, 0, len(msgs)+1)
	out = append(out, System(prompt))
	out = append(out, msgs...)
	return out
}
