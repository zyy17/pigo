package ai

import "encoding/json"

// These types are the flat view of one chat-completions SSE payload.
// The wire format nests "delta" and "function". Parsing lifts those
// fields up so the rest of the client never walks a struct tree.

type chunkError struct {
	Message string `json:"message"`
}

type chunkFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// chunkCallWire is the one-level JSON shape of a tool-call delta.
type chunkCallWire struct {
	Index    int           `json:"index"`
	ID       string        `json:"id"`
	Function chunkFunction `json:"function"`
}

// chunkCall is a tool-call delta with the function fields lifted out.
type chunkCall struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

func (c *chunkCall) UnmarshalJSON(data []byte) error {
	var raw chunkCallWire
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Index = raw.Index
	c.ID = raw.ID
	c.Name = raw.Function.Name
	c.Arguments = raw.Function.Arguments
	return nil
}

type chunkChoiceWire struct {
	Delta        json.RawMessage `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

type chunkDelta struct {
	Content   string            `json:"content"`
	ToolCalls []json.RawMessage `json:"tool_calls"`
}

type chunkEnvelope struct {
	Choices []chunkChoiceWire `json:"choices"`
	Error   *chunkError       `json:"error"`
}

// parsedChunk is one SSE payload after nesting has been removed.
type parsedChunk struct {
	Content      string
	Calls        []chunkCall
	FinishReason string
	ErrorMessage string
}

func parseChunk(data string) (parsedChunk, error) {
	var env chunkEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return parsedChunk{}, err
	}
	out := parsedChunk{}
	if env.Error != nil {
		out.ErrorMessage = env.Error.Message
	}
	if len(env.Choices) == 0 {
		return out, nil
	}
	choice := env.Choices[0]
	if choice.FinishReason != nil {
		out.FinishReason = *choice.FinishReason
	}
	if len(choice.Delta) == 0 || string(choice.Delta) == "null" {
		return out, nil
	}
	var delta chunkDelta
	if err := json.Unmarshal(choice.Delta, &delta); err != nil {
		return parsedChunk{}, err
	}
	out.Content = delta.Content
	for _, raw := range delta.ToolCalls {
		var call chunkCall
		if err := json.Unmarshal(raw, &call); err != nil {
			return parsedChunk{}, err
		}
		out.Calls = append(out.Calls, call)
	}
	return out, nil
}
