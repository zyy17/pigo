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
	start := s.compaction.From
	if s.compaction.FirstKeptID != "" {
		if i := s.index(s.compaction.FirstKeptID); i >= 0 {
			start = i
		}
	}
	if start < 0 {
		start = 0
	}
	if start > len(s.Messages) {
		start = len(s.Messages)
	}
	return s.pairedStart(start)
}

// pairedStart keeps a tool result with the assistant that produced it.
// The kept id can name the result when an earlier session line never landed.
// A result with no call just before it is not sent.
func (s State) pairedStart(start int) int {
	if start < 0 || start >= len(s.Messages) || s.Messages[start].Role != ai.RoleToolResult {
		return start
	}
	i := start
	for i > 0 && s.Messages[i-1].Role == ai.RoleToolResult {
		i--
	}
	if i > 0 && s.Messages[i-1].Role == ai.RoleAssistant {
		return i - 1
	}
	for start < len(s.Messages) && s.Messages[start].Role == ai.RoleToolResult {
		start++
	}
	return start
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
