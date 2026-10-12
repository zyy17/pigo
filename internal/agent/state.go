package agent

import "github.com/zyy17/pigo/internal/ai"

// State is the loop's memory: the prompt, the model, the tools, and the transcript.
// Pi keeps the same four pieces on Agent.state.
type State struct {
	SystemPrompt string
	Model        string
	Tools        []Tool
	Messages     []ai.Message
}

func (s State) snapshot() []ai.Message {
	out := make([]ai.Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

func (s State) tool(name string) (Tool, bool) {
	for _, tool := range s.Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

func (s State) schemas() []ai.Tool {
	out := make([]ai.Tool, 0, len(s.Tools))
	for _, tool := range s.Tools {
		out = append(out, tool.Schema())
	}
	return out
}
