package agent

import (
	"github.com/zyy17/pigo/internal/ai"
)

// EventType is one lifecycle event. The names follow Pi's agent events.
type EventType string

const (
	AgentStart    EventType = "agent_start"
	AgentEnd      EventType = "agent_end"
	TurnStart     EventType = "turn_start"
	TurnEnd       EventType = "turn_end"
	MessageStart  EventType = "message_start"
	MessageUpdate EventType = "message_update"
	MessageEnd    EventType = "message_end"
	ToolStart     EventType = "tool_execution_start"
	ToolEnd       EventType = "tool_execution_end"
)

// Event reports loop progress to the UI.
// Message is set for message events. Tool fields are set for tool events.
type Event struct {
	Type      EventType
	Turn      int
	Message   ai.Message
	TextDelta string
	ToolID    string
	ToolName  string
	Args      []byte
	Result    string
	IsError   bool
	Err       error
}
