package agent

import (
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/ai"
)

func TestEstimateAndThreshold(t *testing.T) {
	if estimateText("") != 0 || estimateText("a") != 1 || estimateText("abcd") != 1 || estimateText("abcde") != 2 {
		t.Fatalf("empty %d one %d four %d five %d", estimateText(""), estimateText("a"), estimateText("abcd"), estimateText("abcde"))
	}
	if shouldCompact(40, 50, 10) {
		t.Fatal("estimate equal to the reserve line compacted")
	}
	if !shouldCompact(41, 50, 10) {
		t.Fatal("one token past the reserve line did not compact")
	}
	if !shouldCompact(1, 10, 20) {
		t.Fatal("reserve larger than the window did not compact")
	}
	got := DefaultCompactSettings()
	if !got.Enabled || got.ContextWindow != 128_000 || got.ReserveTokens != 16_384 || got.KeepRecentTokens != 20_000 {
		t.Fatalf("%#v", got)
	}
	if zero := (CompactSettings{}).orDefault(); zero != got {
		t.Fatalf("zero settings %#v", zero)
	}
}

func TestFindCutKeepsTheToolPair(t *testing.T) {
	user := ai.User(strings.Repeat("a", 40*4))
	call := ai.Assistant("", ai.StopToolUse, ai.ToolCall{
		ID: "c", Name: "read_file", Arguments: []byte(`{"path":"main.go"}`),
	})
	result := ai.ToolResult("c", "read_file", strings.Repeat("b", 80*4), false)
	done := ai.Assistant(strings.Repeat("c", 20*4), ai.StopStop)

	cut, ok := findCut([]ai.Message{user, call, result, done}, 15)
	if !ok || cut != 3 {
		t.Fatalf("forward cut %d ok %v", cut, ok)
	}
	cut, ok = findCut([]ai.Message{user, call, result}, 10)
	if !ok || cut != 1 {
		t.Fatalf("fallback cut %d ok %v", cut, ok)
	}
	if _, ok := findCut([]ai.Message{user, call, result, done}, 10_000); ok {
		t.Fatal("a short span was cut")
	}
	if _, ok := findCut([]ai.Message{result}, 1); ok {
		t.Fatal("a lone tool result was a cut")
	}
}

func TestSerializeTruncatesToolResults(t *testing.T) {
	text := strings.Repeat("字", toolResultRunes+5)
	got := serializeConversation([]ai.Message{
		ai.User("hello"),
		ai.Assistant("look", ai.StopToolUse, ai.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"a.go"}`)}),
		ai.ToolResult("c", "read_file", text, false),
	})
	if !strings.Contains(got, "[User]: hello") || !strings.Contains(got, `read_file({"path":"a.go"})`) {
		t.Fatalf("serialized:\n%s", got)
	}
	if !strings.Contains(got, "…[5 chars truncated]") {
		t.Fatalf("truncation marker missing:\n%s", got)
	}
	if strings.Contains(got, strings.Repeat("字", toolResultRunes+1)) {
		t.Fatal("tool result was not truncated")
	}
}

func TestFileListsCarryForward(t *testing.T) {
	msgs := []ai.Message{ai.Assistant("", ai.StopToolUse,
		ai.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"main.go"}`)},
		ai.ToolCall{Name: "edit_file", Arguments: []byte(`{"path":"main.go"}`)},
		ai.ToolCall{Name: "write_file", Arguments: []byte(`{"path":"new.go"}`)},
		ai.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"main.go"}`)},
	)}
	read, modified := collectFiles(msgs, []string{"old.go"}, []string{"old.go"})
	if strings.Join(read, ",") != "old.go,main.go" {
		t.Fatalf("read %v", read)
	}
	if strings.Join(modified, ",") != "old.go,main.go,new.go" {
		t.Fatalf("modified %v", modified)
	}
	summary := appendFileLists("## Goal\nkeep", read, nil)
	if !strings.Contains(summary, "<read-files>\nold.go\nmain.go\n</read-files>") {
		t.Fatalf("summary:\n%s", summary)
	}
	stripped := stripFileTags(summary + "\n\n<modified-files>\nold.go\n</modified-files>")
	if stripped != "## Goal\nkeep" {
		t.Fatalf("stripped %q", stripped)
	}
}
