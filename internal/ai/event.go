package ai

import "context"

// EventType is one step of a model stream.
type EventType string

const (
	// EventTextDelta is a piece of assistant text.
	EventTextDelta EventType = "text_delta"
	// EventDone carries the finished assistant message.
	EventDone EventType = "done"
	// EventError ends the stream. Message may hold partial output.
	EventError EventType = "error"
)

// Event is one item from StreamFunc.
// Text deltas are for display. The done event is the assistant message
// the agent stores, including any tool calls.
type Event struct {
	Type    EventType
	Text    string
	Message Message
	Err     error
}

// Request is one model call: the transcript, the tool schemas, and the model name.
type Request struct {
	Model    string
	Messages []Message
	Tools    []Tool
}

// StreamFunc sends a request and returns a stream of events.
// The channel must be closed after a done or error event.
// Cancel ctx to abandon the stream.
type StreamFunc func(ctx context.Context, req Request) (<-chan Event, error)
