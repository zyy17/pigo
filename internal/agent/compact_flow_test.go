package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func TestAutoCompactReplacesOlderMessages(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant(tok(20), ai.StopStop),
		ai.Assistant("## Goal\nkeep going", ai.StopStop),
		ai.Assistant("done", ai.StopStop),
	}}
	ag := newCompactAgent(t, script, charBudget())
	if err := ag.Prompt(context.Background(), tok(20)); err != nil {
		t.Fatal(err)
	}
	if len(script.reqs) != 1 {
		t.Fatalf("first prompt compacted: %d calls", len(script.reqs))
	}
	if err := ag.Prompt(context.Background(), tok(20)); err != nil {
		t.Fatal(err)
	}
	if len(script.reqs) != 3 {
		t.Fatalf("calls %d", len(script.reqs))
	}
	summaryReq := script.reqs[1]
	if len(summaryReq.Tools) != 0 || !strings.Contains(summaryReq.Messages[0].Text, "structured summary") {
		t.Fatalf("summary request tools %d system %q", len(summaryReq.Tools), summaryReq.Messages[0].Text)
	}
	if !strings.Contains(summaryReq.Messages[1].Text, "[User]:") || !strings.Contains(summaryReq.Messages[1].Text, "## Goal") {
		t.Fatalf("summary prompt:\n%s", summaryReq.Messages[1].Text)
	}
	chat := script.reqs[2]
	if roles(chat.Messages) != "system,user,user" {
		t.Fatalf("compacted request %s", roles(chat.Messages))
	}
	if !strings.Contains(chat.Messages[1].Text, "<summary>") || !strings.Contains(chat.Messages[1].Text, "## Goal\nkeep going") {
		t.Fatalf("summary message:\n%s", chat.Messages[1].Text)
	}
	if chat.Messages[2].Text != tok(20) {
		t.Fatalf("kept tail %q", chat.Messages[2].Text)
	}
	if strings.Contains(chat.Messages[1].Text, tok(20)) {
		t.Fatal("older user text was copied into the summary message")
	}
	msgs := ag.Messages()
	if len(msgs) != 4 || msgs[0].Text != tok(20) {
		t.Fatalf("transcript lost older messages: %d", len(msgs))
	}
	entry, ok := ag.Compaction()
	if !ok || entry.FirstKeptID == "" || entry.TokensBefore <= 40 {
		t.Fatalf("entry %#v", entry)
	}
}

func TestProviderUsageTriggersCompact(t *testing.T) {
	script := &fakeModel{
		msgs: []ai.Message{
			ai.Assistant("ok", ai.StopStop),
			ai.Assistant("## Goal\nkeep", ai.StopStop),
			ai.Assistant("next", ai.StopStop),
			ai.Assistant("later", ai.StopStop),
		},
		usages: []*ai.Usage{
			{Input: 80, Output: 0, Total: 80},
			{Input: 7, Output: 2, Total: 9},
			nil,
			nil,
		},
	}
	ag := newCompactAgent(t, script, agent.CompactSettings{
		Enabled:          true,
		ContextWindow:    100,
		ReserveTokens:    40,
		KeepRecentTokens: 10,
	})
	var compacted agent.Event
	ag.Subscribe(func(ev agent.Event) {
		if ev.Type == agent.Compacted {
			compacted = ev
		}
	})
	if err := ag.Prompt(context.Background(), tok(30)); err != nil {
		t.Fatal(err)
	}
	if err := ag.Prompt(context.Background(), tok(20)); err != nil {
		t.Fatal(err)
	}
	if len(script.reqs) != 3 || len(script.reqs[1].Tools) != 0 {
		t.Fatalf("usage did not compact: %d calls", len(script.reqs))
	}
	if roles(script.reqs[2].Messages) != "system,user,user" || script.reqs[2].Messages[2].Text != tok(20) {
		t.Fatalf("request %s %q", roles(script.reqs[2].Messages), script.reqs[2].Messages[2].Text)
	}
	if strings.Contains(script.reqs[2].Messages[1].Text, tok(30)) {
		t.Fatal("older user text was still sent")
	}
	if compacted.Usage == nil || compacted.Usage.Input != 7 || compacted.Session == nil || compacted.Session.Input != 87 || compacted.Session.Output != 2 {
		t.Fatalf("compaction usage last %#v session %#v", compacted.Usage, compacted.Session)
	}
	got := ag.Usage()
	if got.HaveLast || !got.HaveSession || got.Session.Input != 87 || got.Session.Output != 2 {
		t.Fatalf("session usage %#v", got)
	}
	if err := ag.Prompt(context.Background(), "CCC"); err != nil {
		t.Fatal(err)
	}
	if len(script.reqs) != 4 || len(script.reqs[3].Tools) == 0 {
		t.Fatal("cleared anchor compacted again")
	}
}

func TestManualCompactFocusAndSecondPass(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("one", ai.StopStop),
		ai.Assistant("two", ai.StopStop),
		ai.Assistant("a", ai.StopStop),
		ai.Assistant("b", ai.StopStop),
		ai.Assistant("c", ai.StopStop),
		ai.Assistant("## Goal\nfirst", ai.StopStop),
		ai.Assistant("d", ai.StopStop),
		ai.Assistant("## Goal\nsecond", ai.StopStop),
		ai.Assistant("fresh", ai.StopStop),
	}}
	ag := newCompactAgent(t, script, manualBudget())
	if err := ag.Prompt(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	if err := ag.Prompt(context.Background(), "there"); err != nil {
		t.Fatal(err)
	}
	if err := ag.Compact(context.Background(), ""); !errors.Is(err, agent.ErrNothingToCompact) {
		t.Fatal(err)
	}
	for _, text := range []string{tok(40), tok(40), tok(40)} {
		if err := ag.Prompt(context.Background(), text); err != nil {
			t.Fatal(err)
		}
	}
	before := len(script.reqs)
	if err := ag.Compact(context.Background(), "The Parser"); err != nil {
		t.Fatal(err)
	}
	focus := script.reqs[before].Messages[1].Text
	if !strings.Contains(focus, "Additional focus: The Parser") || strings.Contains(focus, "<previous-summary>") {
		t.Fatalf("focus prompt:\n%s", focus)
	}
	first, ok := ag.Compaction()
	if !ok || !strings.Contains(first.Summary, "## Goal\nfirst") {
		t.Fatalf("first entry %#v", first)
	}
	if err := ag.Prompt(context.Background(), tok(40)); err != nil {
		t.Fatal(err)
	}
	if err := ag.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	secondReq := script.reqs[len(script.reqs)-1]
	if !strings.Contains(secondReq.Messages[1].Text, "<previous-summary>") || !strings.Contains(secondReq.Messages[1].Text, "## Goal\nfirst") {
		t.Fatalf("update prompt:\n%s", secondReq.Messages[1].Text)
	}
	if strings.Contains(secondReq.Messages[1].Text, "<read-files>") {
		t.Fatalf("file tags were sent back as the previous summary:\n%s", secondReq.Messages[1].Text)
	}
	second, ok := ag.Compaction()
	if !ok || second.FirstKeptID == first.FirstKeptID || !strings.Contains(second.Summary, "## Goal\nsecond") {
		t.Fatalf("second %#v first %s", second, first.FirstKeptID)
	}
	if ag.Messages()[0].Text != "hi" {
		t.Fatal("second compact dropped stored messages")
	}

	ag.Reset()
	if _, ok := ag.Compaction(); ok || len(ag.Messages()) != 0 {
		t.Fatal("reset left compaction behind")
	}
	if err := ag.Prompt(context.Background(), "again"); err != nil {
		t.Fatal(err)
	}
	if roles(script.reqs[len(script.reqs)-1].Messages) != "system,user" {
		t.Fatalf("after reset %s", roles(script.reqs[len(script.reqs)-1].Messages))
	}
}

func TestCompactKeepsToolCallWithResult(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse, call("c1", "read_file", map[string]any{"path": "main.go"})),
		ai.Assistant("", ai.StopToolUse, call("c2", "edit_file", map[string]any{"path": "main.go"})),
		ai.Assistant(tok(20), ai.StopStop),
		ai.Assistant("## Goal\nfiles\n\n## Critical Context\nmain.go", ai.StopStop),
		ai.Assistant("after", ai.StopStop),
	}}
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		return tok(80), nil
	}
	ag, err := agent.New(agent.Config{
		SystemPrompt: "be helpful",
		Model:        "fake",
		Tools:        []agent.Tool{tool},
		Stream:       script.stream,
		Compact:      manualBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ag.Prompt(context.Background(), tok(40)); err != nil {
		t.Fatal(err)
	}
	if err := ag.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	entry, ok := ag.Compaction()
	if !ok {
		t.Fatal("missing entry")
	}
	if strings.Join(entry.ReadFiles, ",") != "main.go" || strings.Join(entry.ModifiedFiles, ",") != "main.go" {
		t.Fatalf("files read %v modified %v", entry.ReadFiles, entry.ModifiedFiles)
	}
	if !strings.Contains(entry.Summary, "<read-files>\nmain.go\n</read-files>") || !strings.Contains(entry.Summary, "<modified-files>\nmain.go\n</modified-files>") {
		t.Fatalf("summary:\n%s", entry.Summary)
	}
	summaryReq := script.reqs[len(script.reqs)-1]
	if !strings.Contains(summaryReq.Messages[1].Text, "[Tool result]:") || !strings.Contains(summaryReq.Messages[1].Text, "read_file") {
		t.Fatalf("summary missed the tool pair:\n%s", summaryReq.Messages[1].Text)
	}
	if err := ag.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	chat := script.reqs[len(script.reqs)-1]
	if roles(chat.Messages) != "system,user,assistant,user" {
		t.Fatalf("kept %s", roles(chat.Messages))
	}
	for _, msg := range chat.Messages {
		if msg.Role == ai.RoleToolResult {
			t.Fatal("tool result was sent without its call")
		}
	}
}

func TestCompactFallbackKeepsTrailingToolResult(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("", ai.StopToolUse, call("c1", "read_file", map[string]any{"path": "a.go"})),
		ai.Assistant("## Goal\nprefix", ai.StopStop),
		ai.Assistant("next", ai.StopStop),
	}}
	tool := echoTool()
	tool.Execute = func(ctx context.Context, args []byte) (string, error) {
		return tok(80), nil
	}
	ag, err := agent.New(agent.Config{
		SystemPrompt: "be helpful",
		Model:        "fake",
		Tools:        []agent.Tool{tool},
		Stream:       script.stream,
		MaxTurns:     1,
		Compact:      manualBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = ag.Prompt(context.Background(), tok(40))
	if err == nil || !strings.Contains(err.Error(), "1") {
		t.Fatal(err)
	}
	if err := ag.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if err := ag.Prompt(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	chat := script.reqs[len(script.reqs)-1]
	if roles(chat.Messages) != "system,user,assistant,toolResult,user" {
		t.Fatalf("pair split: %s", roles(chat.Messages))
	}
	if !strings.Contains(chat.Messages[1].Text, "<summary>") {
		t.Fatalf("missing summary:\n%s", chat.Messages[1].Text)
	}
	if chat.Messages[3].Role != ai.RoleToolResult || chat.Messages[2].Role != ai.RoleAssistant {
		t.Fatal("tool result did not stay with its call")
	}
}

func TestCompactFailureLeavesContext(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant(tok(20), ai.StopStop),
		ai.Assistant("ignored", ai.StopStop),
		ai.Assistant("## Goal\nrecovered", ai.StopStop),
		ai.Assistant("done", ai.StopStop),
	}}
	ag := newCompactAgent(t, script, charBudget())
	if err := ag.Prompt(context.Background(), tok(20)); err != nil {
		t.Fatal(err)
	}
	script.streamErr = errors.New("connection reset")
	err := ag.Prompt(context.Background(), tok(20))
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatal(err)
	}
	if _, ok := ag.Compaction(); ok {
		t.Fatal("failed compact stored a summary")
	}
	if len(ag.Messages()) != 3 {
		t.Fatalf("messages %d", len(ag.Messages()))
	}
	if err := ag.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	chat := script.reqs[len(script.reqs)-1]
	if roles(chat.Messages) != "system,user,user,user" || !strings.Contains(chat.Messages[1].Text, "recovered") {
		t.Fatalf("retry request %s\n%s", roles(chat.Messages), chat.Messages[1].Text)
	}
	if _, ok := ag.Compaction(); !ok {
		t.Fatal("retry did not compact")
	}
}

func TestEmptySummaryDoesNotReplace(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("", ai.StopStop),
	}}
	ag := newCompactAgent(t, script, manualBudget())
	if err := ag.Prompt(context.Background(), tok(40)); err != nil {
		t.Fatal(err)
	}
	if err := ag.Prompt(context.Background(), tok(40)); err != nil {
		t.Fatal(err)
	}
	err := ag.Compact(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "summary was empty") {
		t.Fatal(err)
	}
	if _, ok := ag.Compaction(); ok {
		t.Fatal("empty summary was stored")
	}
}

func TestCompactCancelLeavesContext(t *testing.T) {
	started := make(chan struct{})
	var calls int
	stream := func(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
		calls++
		summary := len(req.Messages) > 0 && strings.Contains(req.Messages[0].Text, "summarization assistant")
		if summary {
			close(started)
			ch := make(chan ai.Event, 1)
			go func() {
				defer close(ch)
				<-ctx.Done()
				ch <- ai.Event{Type: ai.EventError, Err: ctx.Err()}
			}()
			return ch, nil
		}
		ch := make(chan ai.Event, 1)
		ch <- ai.Event{Type: ai.EventDone, Message: ai.Assistant("ok", ai.StopStop)}
		close(ch)
		return ch, nil
	}
	ag, err := agent.New(agent.Config{
		Model:   "fake",
		Tools:   []agent.Tool{echoTool()},
		Stream:  stream,
		Compact: manualBudget(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ag.Prompt(context.Background(), tok(40)); err != nil {
		t.Fatal(err)
	}
	if err := ag.Prompt(context.Background(), tok(40)); err != nil {
		t.Fatal(err)
	}
	go func() {
		<-started
		ag.Abort()
	}()
	err = ag.Compact(context.Background(), "")
	if !errors.Is(err, agent.ErrInterrupted) {
		t.Fatal(err)
	}
	if _, ok := ag.Compaction(); ok {
		t.Fatal("canceled compact stored a summary")
	}
	if calls != 3 {
		t.Fatalf("calls %d", calls)
	}
}

func TestRestoredToolResultBoundaryIncludesTheCall(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{ai.Assistant("ok", ai.StopStop)}}
	ag := newCompactAgent(t, script, manualBudget())
	ag.Restore([]ai.Message{
		ai.User("old"),
		ai.Assistant("", ai.StopToolUse, call("c1", "read_file", map[string]any{"path": "a.go"})),
		ai.ToolResult("c1", "read_file", "package a", false),
	}, []string{"u", "a", "r"}, &agent.Compaction{
		ID: "c", Summary: "## Goal\nship", FirstKeptID: "r", From: 2,
	})
	if err := ag.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	chat := script.reqs[0]
	if roles(chat.Messages) != "system,user,assistant,toolResult,user" {
		t.Fatalf("boundary %s", roles(chat.Messages))
	}
	if len(chat.Messages[2].ToolCalls) != 1 || chat.Messages[2].ToolCalls[0].ID != "c1" || chat.Messages[3].Text != "package a" {
		t.Fatalf("pair %#v %#v", chat.Messages[2], chat.Messages[3])
	}
	if strings.Contains(chat.Messages[1].Text, "old") || chat.Messages[4].Text != "next" {
		t.Fatalf("projection:\n%s", chat.Messages[1].Text)
	}

	script = &fakeModel{msgs: []ai.Message{ai.Assistant("ok", ai.StopStop)}}
	ag = newCompactAgent(t, script, manualBudget())
	ag.Restore([]ai.Message{
		ai.ToolResult("missing", "read_file", "orphan", false),
		ai.User("kept"),
	}, []string{"r", "u"}, &agent.Compaction{
		ID: "c", Summary: "## Goal\nship", FirstKeptID: "r", From: 0,
	})
	if err := ag.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	chat = script.reqs[0]
	if roles(chat.Messages) != "system,user,user,user" || chat.Messages[2].Text != "kept" || chat.Messages[3].Text != "next" {
		t.Fatalf("orphan boundary %s", roles(chat.Messages))
	}
	for _, msg := range chat.Messages {
		if msg.Role == ai.RoleToolResult {
			t.Fatal("orphan tool result was sent")
		}
	}
}

func TestDisabledAutoStillCompactsManually(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("## Goal\nmanual", ai.StopStop),
	}}
	ag := newCompactAgent(t, script, manualBudget())
	if err := ag.Prompt(context.Background(), tok(40)); err != nil {
		t.Fatal(err)
	}
	if err := ag.Prompt(context.Background(), tok(40)); err != nil {
		t.Fatal(err)
	}
	if len(script.reqs) != 2 {
		t.Fatalf("auto compact ran while disabled: %d", len(script.reqs))
	}
	if err := ag.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := ag.Compaction(); !ok {
		t.Fatal("manual compact did not run")
	}
}

func newCompactAgent(t *testing.T, script *fakeModel, settings agent.CompactSettings) *agent.Agent {
	t.Helper()
	ag, err := agent.New(agent.Config{
		SystemPrompt: "be helpful",
		Model:        "fake",
		Tools:        []agent.Tool{echoTool()},
		Stream:       script.stream,
		Compact:      settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ag
}

func charBudget() agent.CompactSettings {
	return agent.CompactSettings{
		Enabled:          true,
		ContextWindow:    50,
		ReserveTokens:    10,
		KeepRecentTokens: 15,
	}
}

func manualBudget() agent.CompactSettings {
	return agent.CompactSettings{
		Enabled:          false,
		ContextWindow:    1000,
		ReserveTokens:    10,
		KeepRecentTokens: 10,
	}
}

func tok(n int) string {
	return strings.Repeat("a", n*4)
}
