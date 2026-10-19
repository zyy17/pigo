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

// Usage is the latest model call and the session sum.
// HaveLast is false when that call omitted a usage object.
// HaveSession is false until any call reports one.
type Usage struct {
	Last        ai.Usage
	Session     ai.Usage
	HaveLast    bool
	HaveSession bool
}

// Event reports loop progress to the UI.
// Message is set for message events. Tool fields are set for tool events.
// Usage is this model call, set on the assistant message_end. Nil when omitted.
// Session is the sum of calls that reported usage. Nil until one does.
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
	Usage     *ai.Usage
	Session   *ai.Usage
}
