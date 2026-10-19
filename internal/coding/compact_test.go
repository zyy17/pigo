package coding

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func TestSessionCompactUsesTheSameStream(t *testing.T) {
	script := &scripted{msgs: []ai.Message{
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("## Goal\nship", ai.StopStop),
		ai.Assistant("next", ai.StopStop),
	}}
	session, err := New(Config{
		Workspace:     t.TempDir(),
		UserSkillsDir: t.TempDir(),
		Model:         "fake",
		Stream:        script.stream,
		Compact: agent.CompactSettings{
			Enabled:          false,
			ContextWindow:    1000,
			ReserveTokens:    10,
			KeepRecentTokens: 8,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("a", 40*4)
	if err := session.Prompt(context.Background(), big); err != nil {
		t.Fatal(err)
	}
	if err := session.Compact(context.Background(), ""); err == nil || !errors.Is(err, agent.ErrNothingToCompact) {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), big); err != nil {
		t.Fatal(err)
	}
	if err := session.Compact(context.Background(), "the parser"); err != nil {
		t.Fatal(err)
	}
	reqs := script.requests()
	summary := reqs[len(reqs)-1]
	if len(summary.Tools) != 0 || !strings.Contains(summary.Messages[1].Text, "Additional focus: the parser") {
		t.Fatalf("summary request tools %d", len(summary.Tools))
	}
	if err := session.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	reqs = script.requests()
	chat := reqs[len(reqs)-1]
	if !strings.Contains(chat.Messages[1].Text, "## Goal\nship") || len(chat.Tools) == 0 {
		t.Fatalf("next prompt did not send the summary")
	}
}

func TestCompactionLeafRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	old := strings.Repeat("a", 40*4)
	kept := strings.Repeat("b", 40*4)
	script := &scripted{msgs: []ai.Message{
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("## Goal\nship", ai.StopStop),
	}}
	session, err := New(Config{
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		Model:         "fake",
		SessionDir:    sessions,
		Stream:        script.stream,
		Compact: agent.CompactSettings{
			Enabled:          false,
			ContextWindow:    1000,
			ReserveTokens:    10,
			KeepRecentTokens: 8,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), kept); err != nil {
		t.Fatal(err)
	}
	if err := session.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(session.SessionFile())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), old) {
		t.Fatal("raw transcript dropped the summarized message")
	}
	lines := fileEntries(t, raw)
	last := lines[len(lines)-1]
	if last.Type != "compaction" || last.FirstKeptEntryID == "" || last.ParentID == nil {
		t.Fatalf("leaf %#v", last)
	}
	if !strings.Contains(last.Summary, "## Goal\nship") || last.TokensBefore <= 0 {
		t.Fatalf("summary entry %#v", last)
	}
	id := session.SessionID()
	session.closeJournal()

	next := &scripted{msgs: []ai.Message{ai.Assistant("after", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, id, next)
	if !containsText(resumed.Messages(), old) || !containsText(resumed.Messages(), kept) {
		t.Fatalf("resume dropped stored messages %s", roleTexts(resumed.Messages()))
	}
	got, ok := resumed.agent.Compaction()
	if !ok || !strings.Contains(got.Summary, "## Goal\nship") || got.FirstKeptID == "" {
		t.Fatalf("restored compaction %#v %v", got, ok)
	}
	if err := resumed.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	req := next.requests()[0]
	if roleList(req.Messages) != "system,user,user,assistant,user" {
		t.Fatalf("resumed request %s", roleList(req.Messages))
	}
	if !strings.Contains(req.Messages[1].Text, "<summary>") || !strings.Contains(req.Messages[1].Text, "## Goal\nship") {
		t.Fatalf("summary was not sent:\n%s", req.Messages[1].Text)
	}
	if strings.Contains(texts(req.Messages), old) || !strings.Contains(texts(req.Messages), kept) {
		t.Fatal("resumed request sent the summarized user or dropped the tail")
	}
	again, err := os.ReadFile(resumed.SessionFile())
	if err != nil {
		t.Fatal(err)
	}
	var parent string
	for _, line := range fileEntries(t, again) {
		if line.Type != "message" || !strings.Contains(string(line.Message), "continue") {
			continue
		}
		if line.ParentID == nil {
			t.Fatal("continued message has no parent")
		}
		parent = *line.ParentID
	}
	if parent != last.ID {
		t.Fatalf("continued parent %s, compaction %s", parent, last.ID)
	}
}

func TestHandWrittenCompactionProjectsTheTail(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	path := filepath.Join(sessionBucket(sessions, root), "compact.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file := strings.Join([]string{
		`{"type":"session","version":3,"id":"compact-session","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}`,
		`{"type":"message","id":"old","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"drop me","timestamp":1}}`,
		`{"type":"message","id":"kept","parentId":"old","timestamp":"2026-10-19T00:00:02.000Z","message":{"role":"user","content":"keep me","timestamp":2}}`,
		`{"type":"compaction","id":"comp","parentId":"kept","timestamp":"2026-10-19T00:00:03.000Z","summary":"## Goal\nship","firstKeptEntryId":"kept","tokensBefore":40}`,
		`{"type":"message","id":"later","parentId":"comp","timestamp":"2026-10-19T00:00:04.000Z","message":{"role":"user","content":"after compact","timestamp":4}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	script := &scripted{msgs: []ai.Message{ai.Assistant("ok", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, path, script)
	if roleTexts(resumed.Messages()) != "user:drop me,user:keep me,user:after compact" {
		t.Fatalf("stored %s", roleTexts(resumed.Messages()))
	}
	if err := resumed.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	req := script.requests()[0]
	if roleList(req.Messages) != "system,user,user,user,user" {
		t.Fatalf("request %s", roleList(req.Messages))
	}
	if !strings.Contains(req.Messages[1].Text, "## Goal\nship") || strings.Contains(texts(req.Messages), "drop me") {
		t.Fatalf("projection:\n%s", texts(req.Messages))
	}
	if !strings.Contains(texts(req.Messages), "keep me") || !strings.Contains(texts(req.Messages), "after compact") {
		t.Fatalf("tail missing:\n%s", texts(req.Messages))
	}
}

func TestMissingKeptIDStartsAfterTheCompaction(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	path := filepath.Join(sessionBucket(sessions, root), "missing.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file := strings.Join([]string{
		`{"type":"session","version":3,"id":"missing-kept","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}`,
		`{"type":"message","id":"old","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"drop-this-turn","timestamp":1}}`,
		`{"type":"compaction","id":"comp","parentId":"old","timestamp":"2026-10-19T00:00:02.000Z","summary":"## Goal\nrecovered","firstKeptEntryId":"missing","tokensBefore":10}`,
		`{"type":"message","id":"later","parentId":"comp","timestamp":"2026-10-19T00:00:03.000Z","message":{"role":"user","content":"after","timestamp":3}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	script := &scripted{msgs: []ai.Message{ai.Assistant("ok", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, path, script)
	if err := resumed.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	body := texts(script.requests()[0].Messages)
	if strings.Contains(body, "drop-this-turn") || !strings.Contains(body, "after") || !strings.Contains(body, "## Goal\nrecovered") {
		t.Fatalf("fallback projection:\n%s", body)
	}
}

func TestSideBranchCompactionIsNotApplied(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	path := filepath.Join(sessionBucket(sessions, root), "branch.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file := strings.Join([]string{
		`{"type":"session","version":3,"id":"branch-session","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}`,
		`{"type":"message","id":"user","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"on the leaf","timestamp":1}}`,
		`{"type":"message","id":"side","parentId":"user","timestamp":"2026-10-19T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"off-path branch"}],"stopReason":"stop","timestamp":2}}`,
		`{"type":"compaction","id":"comp","parentId":"side","timestamp":"2026-10-19T00:00:03.000Z","summary":"## Goal\noff-path","firstKeptEntryId":"side","tokensBefore":5}`,
		`{"type":"message","id":"leaf","parentId":"user","timestamp":"2026-10-19T00:00:04.000Z","message":{"role":"assistant","content":[{"type":"text","text":"leaf answer"}],"stopReason":"stop","timestamp":4}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	script := &scripted{msgs: []ai.Message{ai.Assistant("ok", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, path, script)
	if err := resumed.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	body := texts(script.requests()[0].Messages)
	if strings.Contains(body, "<summary>") || strings.Contains(body, "off-path") || !strings.Contains(body, "on the leaf") || !strings.Contains(body, "leaf answer") {
		t.Fatalf("side compaction leaked:\n%s", body)
	}
}

func TestCompactLeavesFileReviewInPlace(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	edit := ai.Assistant("", ai.StopToolUse, ai.ToolCall{
		ID:        "e1",
		Name:      editToolName,
		Arguments: []byte(`{"path":"main.go","old_string":"package main\n","new_string":"package pigo\n"}`),
	})
	script := &scripted{msgs: []ai.Message{
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("## Goal\nedit", ai.StopStop),
		edit,
		ai.Assistant("done", ai.StopStop),
	}}
	session := newCompactSession(t, dir, sessions, false, script)
	if err := os.WriteFile(filepath.Join(session.Root(), "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	asked := 0
	session.SetConfirm(func(_ context.Context, prompt Confirm) (bool, error) {
		asked++
		if !strings.Contains(prompt.Title, "edit") || prompt.Detail != "main.go" {
			t.Fatalf("confirm %#v", prompt)
		}
		return true, nil
	})
	old := strings.Repeat("a", 40*4)
	if err := session.Prompt(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if err := session.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "edit the file"); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Fatalf("confirm calls %d", asked)
	}
	body, err := os.ReadFile(filepath.Join(session.Root(), "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "package pigo\n" {
		t.Fatalf("file %q", body)
	}

	dir = t.TempDir()
	yoloScript := &scripted{msgs: []ai.Message{
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("## Goal\nedit", ai.StopStop),
		edit,
		ai.Assistant("done", ai.StopStop),
	}}
	yolo := newCompactSession(t, dir, t.TempDir(), true, yoloScript)
	if err := os.WriteFile(filepath.Join(yolo.Root(), "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	yolo.SetConfirm(func(context.Context, Confirm) (bool, error) {
		t.Fatal("yolo asked")
		return false, nil
	})
	if err := yolo.Prompt(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if err := yolo.Prompt(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if err := yolo.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if err := yolo.Prompt(context.Background(), "edit the file"); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(filepath.Join(yolo.Root(), "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "package pigo\n" {
		t.Fatalf("yolo file %q", body)
	}
}

func TestFailedWriteDoesNotShiftCompactionKeptID(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	notes := strings.Repeat("n", 400)
	script := &scripted{msgs: []ai.Message{
		ai.Assistant("ok", ai.StopStop),
		toolMsg("call_r", "read_file", map[string]any{"path": "notes.txt"}),
		ai.Assistant("## Goal\nship", ai.StopStop),
	}}
	session, err := New(Config{
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		Model:         "fake",
		SessionDir:    sessions,
		MaxTurns:      1,
		Stream:        script.stream,
		Compact: agent.CompactSettings{
			Enabled:          false,
			ContextWindow:    1000,
			ReserveTokens:    10,
			KeepRecentTokens: 8,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.closeJournal)
	if err := os.WriteFile(filepath.Join(session.Root(), "notes.txt"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	early := "drop-early-" + strings.Repeat("a", 80)
	if err := session.Prompt(context.Background(), early); err != nil {
		t.Fatal(err)
	}
	restore := blockJournal(t, session.journal)
	if err := session.Prompt(context.Background(), "never-written"); err == nil {
		t.Fatal("session write succeeded")
	}
	restore()
	err = session.Prompt(context.Background(), "read the notes")
	if err == nil || !strings.Contains(err.Error(), "1") {
		t.Fatal(err)
	}
	if err := session.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	lines := fileEntries(t, mustRead(t, session.SessionFile()))
	var toolID, resultID, keptID string
	for _, line := range lines {
		if line.Type == "compaction" {
			keptID = line.FirstKeptEntryID
			continue
		}
		msg, skip, err := decodeMessage(line.Message)
		if err != nil {
			t.Fatal(err)
		}
		if skip {
			continue
		}
		if msg.Role == ai.RoleAssistant && len(msg.ToolCalls) > 0 {
			toolID = line.ID
		}
		if msg.Role == ai.RoleToolResult {
			resultID = line.ID
		}
	}
	if keptID == "" || keptID == resultID || keptID != toolID {
		t.Fatalf("firstKept %s tool %s result %s", keptID, toolID, resultID)
	}
	id := session.SessionID()
	session.closeJournal()

	next := &scripted{msgs: []ai.Message{ai.Assistant("after", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, id, next)
	if err := resumed.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	req := next.requests()[0]
	if roleList(req.Messages) != "system,user,assistant,toolResult,user" {
		t.Fatalf("resumed request %s", roleList(req.Messages))
	}
	if len(req.Messages[2].ToolCalls) != 1 || req.Messages[2].ToolCalls[0].ID != "call_r" {
		t.Fatalf("call %#v", req.Messages[2])
	}
	if !strings.Contains(req.Messages[3].Text, "nnnn") || strings.Contains(texts(req.Messages), "drop-early") || strings.Contains(texts(req.Messages), "never-written") {
		t.Fatalf("tail:\n%s", texts(req.Messages))
	}
}

func TestCompactRefusesAnUnwrittenKeptMessage(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	script := &scripted{msgs: []ai.Message{
		ai.Assistant("ok", ai.StopStop),
		ai.Assistant("## Goal\nnope", ai.StopStop),
		ai.Assistant("still here", ai.StopStop),
	}}
	session := newCompactSession(t, dir, sessions, false, script)
	if err := session.Prompt(context.Background(), strings.Repeat("a", 40*4)); err != nil {
		t.Fatal(err)
	}
	restore := blockJournal(t, session.journal)
	kept := strings.Repeat("b", 80*4)
	if err := session.Prompt(context.Background(), kept); err == nil {
		t.Fatal("session write succeeded")
	}
	restore()
	if err := session.Compact(context.Background(), ""); err == nil {
		t.Fatal("compact recorded a boundary that is not in the file")
	}
	if _, ok := session.agent.Compaction(); ok {
		t.Fatal("summary stayed after the session write failed")
	}
	if strings.Contains(string(mustRead(t, session.SessionFile())), `"type":"compaction"`) {
		t.Fatal("compaction line was written")
	}
	if err := session.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	req := script.requests()[len(script.requests())-1]
	if roleList(req.Messages) != "system,user,assistant,user,user" {
		t.Fatalf("request %s", roleList(req.Messages))
	}
	if strings.Contains(texts(req.Messages), "<summary>") || !strings.Contains(texts(req.Messages), kept) {
		t.Fatalf("context changed:\n%s", texts(req.Messages))
	}
}

func TestCompactionKeptOnToolResultIncludesTheCall(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	path := filepath.Join(sessionBucket(sessions, root), "pair.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file := strings.Join([]string{
		`{"type":"session","version":3,"id":"pair-session","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}`,
		`{"type":"message","id":"old","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"drop me","timestamp":1}}`,
		`{"type":"message","id":"a1","parentId":"old","timestamp":"2026-10-19T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"keep","name":"read_file","arguments":{"path":"a.go"}}],"stopReason":"toolUse","timestamp":2}}`,
		`{"type":"message","id":"r1","parentId":"a1","timestamp":"2026-10-19T00:00:03.000Z","message":{"role":"toolResult","toolCallId":"keep","toolName":"read_file","content":[{"type":"text","text":"package a"}],"isError":false,"timestamp":3}}`,
		`{"type":"compaction","id":"comp","parentId":"r1","timestamp":"2026-10-19T00:00:04.000Z","summary":"## Goal\nship","firstKeptEntryId":"r1","tokensBefore":40}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	script := &scripted{msgs: []ai.Message{ai.Assistant("ok", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, path, script)
	if err := resumed.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	req := script.requests()[0]
	if roleList(req.Messages) != "system,user,assistant,toolResult,user" {
		t.Fatalf("request %s", roleList(req.Messages))
	}
	if len(req.Messages[2].ToolCalls) != 1 || req.Messages[2].ToolCalls[0].ID != "keep" || req.Messages[3].Text != "package a" {
		t.Fatalf("pair %#v %#v", req.Messages[2], req.Messages[3])
	}
	if strings.Contains(texts(req.Messages), "drop me") || !strings.Contains(req.Messages[1].Text, "## Goal\nship") {
		t.Fatalf("projection:\n%s", texts(req.Messages))
	}
}

func blockJournal(t *testing.T, j *journal) func() {
	t.Helper()
	if j == nil || j.file == nil {
		t.Fatal("journal has no file")
	}
	// Read-only, so the next append fails and the session file is left as it was.
	blocked, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	real := j.file
	j.file = blocked
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		j.file = real
		_ = blocked.Close()
	}
	t.Cleanup(restore)
	return restore
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newCompactSession(t *testing.T, dir, sessions string, yolo bool, script *scripted) *Session {
	t.Helper()
	session, err := New(Config{
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		Model:         "fake",
		SessionDir:    sessions,
		Yolo:          yolo,
		Stream:        script.stream,
		Compact: agent.CompactSettings{
			Enabled:          false,
			ContextWindow:    1000,
			ReserveTokens:    10,
			KeepRecentTokens: 8,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.closeJournal)
	return session
}

func fileEntries(t *testing.T, raw []byte) []sessionEntry {
	t.Helper()
	var out []sessionEntry
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry sessionEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Type == "session" {
			continue
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		t.Fatal("session file has no entries")
	}
	return out
}
