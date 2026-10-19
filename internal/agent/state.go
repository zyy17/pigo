package agent

import "github.com/zyy17/pigo/internal/ai"

// State is the loop's memory: the prompt, the model, the tools, and the transcript.
// Pi keeps the same four pieces on Agent.state. ids[i] names Messages[i].
// A resumed session uses the file's entry ids. Messages added in this process
// use m1, m2, and so on. The session file stores its own id for each message
// and points a compaction entry at the first one that stays verbatim.
type State struct {
	SystemPrompt string
	Model        string
	Tools        []Tool
	Messages     []ai.Message
	ids          []string
	msgSeq       int
	compactSeq   int
	compaction   *Compaction
}

func (s State) liveStart() int {
	if s.compaction == nil {
		return 0
	}
	if s.compaction.FirstKeptID != "" {
		if i := s.index(s.compaction.FirstKeptID); i >= 0 {
			return i
		}
	}
	if s.compaction.From < 0 {
		return 0
	}
	if s.compaction.From > len(s.Messages) {
		return len(s.Messages)
	}
	return s.compaction.From
}

func (s State) index(id string) int {
	for i, got := range s.ids {
		if got == id {
			return i
		}
	}
	return -1
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
