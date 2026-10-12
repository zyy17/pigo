package agent

import (
	"github.com/zyy17/pigo/internal/ai"
)

// Type is one lifecycle event. The names follow Pi's agent events.
type Type string

const (
	AgentStart    Type = "agent_start"
	AgentEnd      Type = "agent_end"
	TurnStart     Type = "turn_start"
	TurnEnd       Type = "turn_end"
	MessageStart  Type = "message_start"
	MessageUpdate Type = "message_update"
	MessageEnd    Type = "message_end"
	ToolStart     Type = "tool_execution_start"
	ToolEnd       Type = "tool_execution_end"
)

// Event reports loop progress to the UI.
// Message is set for message events. Tool fields are set for tool events.
type Event struct {
	Type      Type
	Turn      int
	Message   ai.Message
	Messages  []ai.Message
	TextDelta string
	ToolID    string
	ToolName  string
	Args      []byte
	Result    string
	IsError   bool
	Err       error
}
