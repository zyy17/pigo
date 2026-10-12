package ai

import "strings"

// Role is who produced a transcript message.
// Tool results are a separate role, matching Pi, and become OpenAI
// "tool" messages only at the HTTP boundary.
type Role string

const (
	RoleSystem     Role = "system"
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "toolResult"
)

// StopReason is why the assistant message ended.
type StopReason string

const (
	StopStop    StopReason = "stop"
	StopToolUse StopReason = "toolUse"
	StopLength  StopReason = "length"
	StopError   StopReason = "error"
	StopAborted StopReason = "aborted"
)

// BlockKind selects which fields of a Block are set.
type BlockKind string

const (
	BlockText     BlockKind = "text"
	BlockToolCall BlockKind = "toolCall"
)

// ToolCall is one function call requested by the model.
// Arguments is the raw JSON object the model produced.
type ToolCall struct {
	ID        string
	Name      string
	Arguments []byte
}

// Block is one piece of a message. Pi uses the same idea for text,
// tool calls, and other model output. pigo keeps text and tool calls.
type Block struct {
	Kind     BlockKind
	Text     string
	ToolCall ToolCall
}

// Message is one transcript entry.
// ToolCallID, ToolName, and IsError are set only on tool results.
type Message struct {
	Role       Role
	Blocks     []Block
	ToolCallID string
	ToolName   string
	IsError    bool
	StopReason StopReason
	Error      string
}

// TextBlock builds a text block.
func TextBlock(text string) Block {
	return Block{Kind: BlockText, Text: text}
}

// CallBlock builds a tool-call block.
func CallBlock(call ToolCall) Block {
	return Block{Kind: BlockToolCall, ToolCall: call}
}

// System builds a system message.
func System(text string) Message {
	return Message{Role: RoleSystem, Blocks: []Block{TextBlock(text)}}
}

// User builds a user message.
func User(text string) Message {
	return Message{Role: RoleUser, Blocks: []Block{TextBlock(text)}}
}

// Assistant builds an assistant message from text and tool calls.
func Assistant(text string, reason StopReason, calls ...ToolCall) Message {
	blocks := make([]Block, 0, 1+len(calls))
	if text != "" {
		blocks = append(blocks, TextBlock(text))
	}
	for _, call := range calls {
		blocks = append(blocks, CallBlock(call))
	}
	return Message{Role: RoleAssistant, Blocks: blocks, StopReason: reason}
}

// ToolResult builds the message that answers one tool call.
func ToolResult(id, name, text string, isError bool) Message {
	return Message{
		Role:       RoleToolResult,
		Blocks:     []Block{TextBlock(text)},
		ToolCallID: id,
		ToolName:   name,
		IsError:    isError,
	}
}

// Text joins the text blocks on a message.
func (m Message) Text() string {
	var b strings.Builder
	for _, block := range m.Blocks {
		if block.Kind == BlockText {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

// ToolCalls returns the tool calls on an assistant message.
func (m Message) ToolCalls() []ToolCall {
	var calls []ToolCall
	for _, block := range m.Blocks {
		if block.Kind == BlockToolCall {
			calls = append(calls, block.ToolCall)
		}
	}
	return calls
}

// WithSystem prepends a system message when prompt is non-empty.
// The returned slice is new; msgs is left as it was.
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
