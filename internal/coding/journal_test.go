package coding

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zyy17/pigo/internal/ai"
)

func TestJournalRoundTripAndResume(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	script := &scripted{msgs: []ai.Message{
		toolMsg("call_w", "write_file", map[string]any{"path": "hello.txt", "content": "hi\n"}),
		ai.Assistant("wrote it", ai.StopStop),
	}}
	session := openPersisted(t, dir, sessions, "", script)
	// This test records a real write. There is no confirm screen, so yolo skips the review.
	session.yolo = true
	if session.SessionFile() != "" {
		t.Fatal("file existed before the first message")
	}
	if err := session.Prompt(context.Background(), "write hello <file>"); err != nil {
		t.Fatal(err)
	}
	file := session.SessionFile()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `\u003c`) || !strings.Contains(string(body), "write hello <file>") {
		t.Fatalf("html was escaped:\n%s", body)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %o", info.Mode().Perm())
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 5 {
		t.Fatalf("lines %d\n%s", len(lines), body)
	}
	var header sessionHeader
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header.Type != "session" || header.Version != sessionVersion || header.Cwd != session.Root() || header.ID != session.SessionID() {
		t.Fatalf("header %#v", header)
	}
	var first sessionEntry
	if err := json.Unmarshal([]byte(lines[1]), &first); err != nil {
		t.Fatal(err)
	}
	if first.Type != "message" || first.ParentID != nil {
		t.Fatalf("root entry %#v", first)
	}
	bucket := sessionBucket(sessions, session.Root())
	bucketInfo, err := os.Stat(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if bucketInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %o", bucketInfo.Mode().Perm())
	}

	next := &scripted{msgs: []ai.Message{ai.Assistant("still hi", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, ResumeLatest, next)
	if resumed.SessionID() != session.SessionID() {
		t.Fatalf("id %s, want %s", resumed.SessionID(), session.SessionID())
	}
	got := roleTexts(resumed.Messages())
	for _, want := range []string{"user:write hello <file>", "toolResult:wrote hello.txt (", "assistant:wrote it"} {
		if !strings.Contains(got, want) {
			t.Fatalf("restored %s, missing %s", got, want)
		}
	}
	if err := resumed.Prompt(context.Background(), "and now"); err != nil {
		t.Fatal(err)
	}
	req := next.requests()[0]
	if !strings.Contains(roleList(req.Messages), "user,assistant,toolResult,assistant,user") {
		t.Fatalf("model saw %s", roleList(req.Messages))
	}
	if !strings.Contains(texts(req.Messages), "write hello <file>") || !strings.Contains(texts(req.Messages), "wrote it") {
		t.Fatalf("prior context missing: %#v", req.Messages)
	}
	if strings.Count(roleList(req.Messages), "system") != 1 {
		t.Fatalf("system copies: %s", roleList(req.Messages))
	}
}

func TestResetStartsANewFileAndKeepsTheOldOne(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	script := &scripted{msgs: []ai.Message{ai.Assistant("one", ai.StopStop), ai.Assistant("two", ai.StopStop)}}
	session := openPersisted(t, dir, sessions, "", script)
	if err := session.Prompt(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	oldFile := session.SessionFile()
	oldID := session.SessionID()
	if err := session.Reset(); err != nil {
		t.Fatal(err)
	}
	if session.SessionFile() != "" {
		t.Fatal("reset created a file before the next message")
	}
	if len(session.Messages()) != 0 {
		t.Fatal("transcript was not cleared")
	}
	// No new message yet, so continue still finds the previous file.
	continued := openPersisted(t, dir, sessions, ResumeLatest, &scripted{msgs: []ai.Message{ai.Assistant("x", ai.StopStop)}})
	if continued.SessionID() != oldID {
		t.Fatalf("continue after a quiet reset opened %s", continued.SessionID())
	}
	continued.closeJournal()

	if err := session.Prompt(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}
	if session.SessionFile() == oldFile || session.SessionID() == oldID {
		t.Fatal("reset kept writing the old file")
	}
	oldBody, err := os.ReadFile(oldFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(oldBody), "first") || strings.Contains(string(oldBody), "second") {
		t.Fatalf("old file\n%s", oldBody)
	}
	req := script.requests()[1]
	if roleList(req.Messages) != "system,user" || req.Messages[1].Text != "second" {
		t.Fatalf("after reset the model saw %#v", req.Messages)
	}

	latest := openPersisted(t, dir, sessions, ResumeLatest, &scripted{msgs: []ai.Message{ai.Assistant("x", ai.StopStop)}})
	if latest.SessionID() != session.SessionID() {
		t.Fatalf("latest %s, want %s", latest.SessionID(), session.SessionID())
	}
	byID := openPersisted(t, dir, sessions, oldID, &scripted{msgs: []ai.Message{ai.Assistant("x", ai.StopStop)}})
	if byID.SessionID() != oldID || !containsText(byID.Messages(), "first") {
		t.Fatalf("prefix resume %#v", byID.Messages())
	}
}

func TestLeafWalkSkipsSideBranchAndUnknownEntries(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	path := filepath.Join(sessionBucket(sessions, root), "sample.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	parent := "user0001"
	usage := "usage001"
	side := "side0001"
	leaf := "leaf0001"
	file := strings.Join([]string{
		`{"type":"session","version":3,"id":"11111111-1111-4111-8111-111111111111","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}`,
		`{"type":"message","id":"` + parent + `","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"system","content":"old prompt","timestamp":1}}`,
		`{"type":"message","id":"realuser","parentId":"` + parent + `","timestamp":"2026-10-19T00:00:02.000Z","message":{"role":"user","content":"on the leaf","timestamp":2}}`,
		`{"type":"usage","id":"` + usage + `","parentId":"realuser","timestamp":"2026-10-19T00:00:03.000Z","kind":"cache_warm"}`,
		`{"type":"message","id":"` + side + `","parentId":"` + usage + `","timestamp":"2026-10-19T00:00:04.000Z","message":{"role":"assistant","content":[{"type":"text","text":"side branch"}],"stopReason":"stop","timestamp":4}}`,
		`{"type":"message","id":"` + leaf + `","parentId":"` + usage + `","timestamp":"2026-10-19T00:00:05.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"secret"},{"type":"text","text":"leaf answer"}],"stopReason":"stop","timestamp":5}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	resumed := openPersisted(t, dir, sessions, path, &scripted{msgs: []ai.Message{ai.Assistant("ok", ai.StopStop)}})
	got := roleTexts(resumed.Messages())
	if got != "user:on the leaf,assistant:leaf answer" {
		t.Fatalf("leaf path %s", got)
	}
	if err := resumed.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
}

func TestAppendAfterTornTailRemainsReadable(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	script := &scripted{msgs: []ai.Message{ai.Assistant("kept", ai.StopStop)}}
	session := openPersisted(t, dir, sessions, "", script)
	if err := session.Prompt(context.Background(), "remember-apple"); err != nil {
		t.Fatal(err)
	}
	path := session.SessionFile()
	session.closeJournal()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Fatalf("session file did not end with a newline:\n%s", body)
	}
	// A short write can leave a partial object and no newline. Load skips it.
	// The next append must not glue a new object onto that fragment.
	torn := string(body) + `{"type":"message","id":"torn"`
	if err := os.WriteFile(path, []byte(torn), 0o600); err != nil {
		t.Fatal(err)
	}

	next := &scripted{msgs: []ai.Message{ai.Assistant("after", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, path, next)
	if !containsText(resumed.Messages(), "remember-apple") || containsText(resumed.Messages(), `"id":"torn"`) {
		t.Fatalf("load %s", roleTexts(resumed.Messages()))
	}
	if err := resumed.Prompt(context.Background(), "remember-pear"); err != nil {
		t.Fatal(err)
	}
	resumed.closeJournal()

	again := openPersisted(t, dir, sessions, path, &scripted{})
	if !containsText(again.Messages(), "remember-apple") || !containsText(again.Messages(), "remember-pear") || !containsText(again.Messages(), "after") {
		t.Fatalf("after append %s", roleTexts(again.Messages()))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"id":"torn"`) {
		t.Fatalf("torn fragment stayed:\n%s", raw)
	}
	assertOneJSONValuePerLine(t, raw)
}

func TestLiveAppendCutsATornTailOnTheOpenFile(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	script := &scripted{msgs: []ai.Message{
		ai.Assistant("kept", ai.StopStop),
		ai.Assistant("after", ai.StopStop),
	}}
	session := openPersisted(t, dir, sessions, "", script)
	if err := session.Prompt(context.Background(), "remember-apple"); err != nil {
		t.Fatal(err)
	}
	path := session.SessionFile()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The session still holds the file. A short write would leave this shape
	// on that same descriptor; the next append has to cut it first.
	if err := os.WriteFile(path, append(body, []byte(`{"type":"message","id":"torn"`)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "remember-pear"); err != nil {
		t.Fatal(err)
	}
	session.closeJournal()

	again := openPersisted(t, dir, sessions, path, &scripted{})
	if !containsText(again.Messages(), "remember-apple") || !containsText(again.Messages(), "remember-pear") || !containsText(again.Messages(), "after") {
		t.Fatalf("live append %s", roleTexts(again.Messages()))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"id":"torn"`) {
		t.Fatalf("torn fragment stayed:\n%s", raw)
	}
	assertOneJSONValuePerLine(t, raw)
}

func TestAppendAfterMissingNewlineKeepsTheLastLine(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	script := &scripted{msgs: []ai.Message{ai.Assistant("kept", ai.StopStop)}}
	session := openPersisted(t, dir, sessions, "", script)
	if err := session.Prompt(context.Background(), "remember-apple"); err != nil {
		t.Fatal(err)
	}
	path := session.SessionFile()
	session.closeJournal()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The object is complete. Only the trailing newline was lost.
	cut := strings.TrimSuffix(string(body), "\n")
	if strings.HasSuffix(cut, "\n") || cut == string(body) {
		t.Fatalf("expected one trailing newline:\n%s", body)
	}
	if err := os.WriteFile(path, []byte(cut), 0o600); err != nil {
		t.Fatal(err)
	}

	next := &scripted{msgs: []ai.Message{ai.Assistant("after", ai.StopStop)}}
	resumed := openPersisted(t, dir, sessions, path, next)
	if !containsText(resumed.Messages(), "remember-apple") || !containsText(resumed.Messages(), "kept") {
		t.Fatalf("load dropped the finished line %s", roleTexts(resumed.Messages()))
	}
	if err := resumed.Prompt(context.Background(), "remember-pear"); err != nil {
		t.Fatal(err)
	}
	resumed.closeJournal()

	again := openPersisted(t, dir, sessions, path, &scripted{})
	if !containsText(again.Messages(), "remember-apple") || !containsText(again.Messages(), "kept") || !containsText(again.Messages(), "remember-pear") {
		t.Fatalf("after append %s", roleTexts(again.Messages()))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertOneJSONValuePerLine(t, raw)
}

func assertOneJSONValuePerLine(t *testing.T, raw []byte) {
	t.Helper()
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(line))
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("line: %v\n%s", err, line)
		}
		if dec.More() {
			t.Fatalf("glued line %s", line)
		}
	}
}

func TestLegacyLinearFileAndTornTail(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	path := filepath.Join(sessionBucket(sessions, root), "legacy.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"type":"session","version":1,"id":"legacy-session","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}`,
		`{"type":"message","id":"m1","timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"first"}}`,
		`{"type":"message","id":"m2","timestamp":"2026-10-19T00:00:02.000Z","message":{"role":"assistant","content":"second","stopReason":"stop"}}`,
		`{"type":"message","id":"m3","timestamp":"2026-10-19T00:00:03.000Z","message":{"role":"user","content":"third"}}`,
	}, "\n")
	// No trailing newline, and a torn fragment after one, so the reader keeps the complete lines.
	torn := body + "\n{\"type\":\"message\",\"id\":\"torn\""
	if err := os.WriteFile(path, []byte(torn), 0o600); err != nil {
		t.Fatal(err)
	}
	resumed := openPersisted(t, dir, sessions, "legacy-session", &scripted{})
	if roleTexts(resumed.Messages()) != "user:first,assistant:second,user:third" {
		t.Fatalf("legacy %s", roleTexts(resumed.Messages()))
	}
}

func TestDanglingToolCallIsDropped(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	path := filepath.Join(sessionBucket(sessions, root), "tools.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file := strings.Join([]string{
		`{"type":"session","version":3,"id":"tool-session","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}`,
		`{"type":"message","id":"u1","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"do both"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-10-19T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"keep","name":"read_file","arguments":{"path":"a.go"}},{"type":"toolCall","id":"drop","name":"read_file","arguments":{"path":"b.go"}}],"stopReason":"toolUse"}}`,
		`{"type":"message","id":"r1","parentId":"a1","timestamp":"2026-10-19T00:00:03.000Z","message":{"role":"toolResult","toolCallId":"keep","toolName":"read_file","content":[{"type":"text","text":"package a"}],"isError":false}}`,
		`{"type":"message","id":"orphan","parentId":"r1","timestamp":"2026-10-19T00:00:04.000Z","message":{"role":"toolResult","toolCallId":"nobody","toolName":"read_file","content":"nope","isError":true}}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	resumed := openPersisted(t, dir, sessions, "tool-session", &scripted{})
	msgs := resumed.Messages()
	if len(msgs) != 3 {
		t.Fatalf("messages %#v", msgs)
	}
	if len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].ID != "keep" || msgs[1].StopReason != ai.StopToolUse {
		t.Fatalf("assistant %#v", msgs[1])
	}
	if string(msgs[1].ToolCalls[0].Arguments) != `{"path":"a.go"}` {
		t.Fatalf("args %s", msgs[1].ToolCalls[0].Arguments)
	}
	if msgs[2].ToolCallID != "keep" || msgs[2].Text != "package a" {
		t.Fatalf("result %#v", msgs[2])
	}
	resumed.closeJournal()
	script := &scripted{msgs: []ai.Message{ai.Assistant("ok", ai.StopStop)}}
	again := openPersisted(t, dir, sessions, path, script)
	if err := again.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	req := script.requests()[0]
	joined := texts(req.Messages)
	if strings.Contains(joined, "b.go") || strings.Contains(joined, "nope") || !strings.Contains(joined, "package a") {
		t.Fatalf("request texts %s", joined)
	}
}

func TestSessionLoadErrors(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	bucket := sessionBucket(sessions, root)
	if err := os.MkdirAll(bucket, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bucket, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	header := `{"type":"session","version":3,"id":"bad-session","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}` + "\n"
	write("dup.jsonl", header+`{"type":"message","id":"same","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"a"}}`+"\n"+
		`{"type":"message","id":"same","parentId":"same","timestamp":"2026-10-19T00:00:02.000Z","message":{"role":"user","content":"b"}}`+"\n")
	write("cycle.jsonl", header+`{"type":"message","id":"aaaa","parentId":"bbbb","timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"a"}}`+"\n"+
		`{"type":"message","id":"bbbb","parentId":"aaaa","timestamp":"2026-10-19T00:00:02.000Z","message":{"role":"user","content":"b"}}`+"\n")
	write("missing.jsonl", header+`{"type":"message","id":"cccc","parentId":"gone","timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"a"}}`+"\n")
	write("corrupt.jsonl", header+"this is not json\n")
	write("one.jsonl", strings.Replace(header, "bad-session", "aaa11111-1111-4111-8111-111111111111", 1)+
		`{"type":"message","id":"u","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"only"}}`+"\n")
	write("two.jsonl", strings.Replace(header, "bad-session", "aaa22222-2222-4222-8222-222222222222", 1)+
		`{"type":"message","id":"u","parentId":null,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"other"}}`+"\n")

	for _, spec := range []string{"dup.jsonl", "cycle.jsonl", "missing.jsonl", "corrupt.jsonl"} {
		path := filepath.Join(bucket, spec)
		_, err := New(Config{
			Workspace: dir, UserSkillsDir: t.TempDir(), Model: "fake", SessionDir: sessions, Resume: path,
			Stream: func(context.Context, ai.Request) (<-chan ai.Event, error) {
				return nil, context.Canceled
			},
		})
		if err == nil {
			t.Fatalf("load %s succeeded", spec)
		}
	}
	if _, err := New(Config{
		Workspace: dir, UserSkillsDir: t.TempDir(), Model: "fake", SessionDir: sessions, Resume: "no-such-id",
		Stream: func(context.Context, ai.Request) (<-chan ai.Event, error) {
			return nil, context.Canceled
		},
	}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatal(err)
	}
	if _, err := New(Config{
		Workspace: dir, UserSkillsDir: t.TempDir(), Model: "fake", SessionDir: sessions, Resume: "aaa",
		Stream: func(context.Context, ai.Request) (<-chan ai.Event, error) {
			return nil, context.Canceled
		},
	}); err == nil || !strings.Contains(err.Error(), "matches 2") {
		t.Fatal(err)
	}
	got := openPersisted(t, dir, sessions, "aaa11111", &scripted{})
	if !containsText(got.Messages(), "only") {
		t.Fatalf("unique prefix %#v", got.Messages())
	}
}

func TestContinueErrorsWhenTheNewestSessionIsCorrupt(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	bucket := sessionBucket(sessions, root)
	if err := os.MkdirAll(bucket, 0o700); err != nil {
		t.Fatal(err)
	}
	writeAt := func(name, body string, mt time.Time) string {
		t.Helper()
		path := filepath.Join(bucket, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mt, mt); err != nil {
			t.Fatal(err)
		}
		return path
	}
	header := func(id string) string {
		return `{"type":"session","version":3,"id":"` + id + `","timestamp":"2026-10-19T00:00:00.000Z","cwd":"` + jsonEscape(root) + `"}`
	}
	userLine := func(id, parent, text string) string {
		parentJSON := "null"
		if parent != "" {
			parentJSON = `"` + parent + `"`
		}
		return `{"type":"message","id":"` + id + `","parentId":` + parentJSON + `,"timestamp":"2026-10-19T00:00:01.000Z","message":{"role":"user","content":"` + text + `","timestamp":1}}`
	}
	old := time.Now().Add(-2 * time.Hour)
	mid := time.Now().Add(-time.Hour)
	writeAt("older.jsonl", header("older-session")+"\n"+userLine("u", "", "from older")+"\n", old)
	newer := writeAt("newer.jsonl", strings.Join([]string{
		header("newer-session"),
		userLine("u", "", "from newer"),
		"this is not json",
		userLine("v", "u", "after bad"),
		"",
	}, "\n"), time.Now())
	writeAt("notes.jsonl", "not a session\n", mid)

	stream := func(context.Context, ai.Request) (<-chan ai.Event, error) {
		return nil, context.Canceled
	}
	_, err := New(Config{
		Workspace: dir, UserSkillsDir: t.TempDir(), Model: "fake", SessionDir: sessions, Resume: ResumeLatest, Stream: stream,
	})
	if err == nil || !strings.Contains(err.Error(), newer) || strings.Contains(err.Error(), "not found") {
		t.Fatal(err)
	}
	_, err = New(Config{
		Workspace: dir, UserSkillsDir: t.TempDir(), Model: "fake", SessionDir: sessions, Resume: "newer-session", Stream: stream,
	})
	if err == nil || !strings.Contains(err.Error(), newer) {
		t.Fatalf("id lookup hid the corrupt file: %v", err)
	}

	// A non-session file must not block continue once the corrupt session is gone.
	if err := os.Remove(newer); err != nil {
		t.Fatal(err)
	}
	opened := openPersisted(t, dir, sessions, ResumeLatest, &scripted{})
	if opened.SessionID() != "older-session" || !containsText(opened.Messages(), "from older") {
		t.Fatalf("continue %s %s", opened.SessionID(), roleTexts(opened.Messages()))
	}
}

func TestContinueKeepsATornNewestSession(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	script := &scripted{msgs: []ai.Message{ai.Assistant("kept", ai.StopStop)}}
	session := openPersisted(t, dir, sessions, "", script)
	if err := session.Prompt(context.Background(), "remember-apple"); err != nil {
		t.Fatal(err)
	}
	path := session.SessionFile()
	id := session.SessionID()
	session.closeJournal()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, []byte(`{"type":"message","id":"torn"`)...), 0o600); err != nil {
		t.Fatal(err)
	}
	resumed := openPersisted(t, dir, sessions, ResumeLatest, &scripted{})
	if resumed.SessionID() != id || !containsText(resumed.Messages(), "remember-apple") {
		t.Fatalf("torn newest %s %s", resumed.SessionID(), roleTexts(resumed.Messages()))
	}
}

func TestContinueBreaksEqualMtimeByHeaderTime(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	ws := openPersisted(t, dir, sessions, "", &scripted{})
	root := ws.Root()
	ws.closeJournal()
	bucket := sessionBucket(sessions, root)
	if err := os.MkdirAll(bucket, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, id, stamp, text string) {
		t.Helper()
		body := strings.Join([]string{
			`{"type":"session","version":3,"id":"` + id + `","timestamp":"` + stamp + `","cwd":"` + jsonEscape(root) + `"}`,
			`{"type":"message","id":"u","parentId":null,"timestamp":"` + stamp + `","message":{"role":"user","content":"` + text + `","timestamp":1}}`,
			"",
		}, "\n")
		if err := os.WriteFile(filepath.Join(bucket, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The later session has the smaller path, so a path tie-break would open the older one.
	write("z-older.jsonl", "older-session", "2026-10-19T00:00:00.000Z", "from older")
	write("a-newer.jsonl", "newer-session", "2026-10-19T00:00:01.000Z", "from newer")
	now := time.Now()
	for _, name := range []string{"z-older.jsonl", "a-newer.jsonl"} {
		path := filepath.Join(bucket, name)
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatal(err)
		}
	}
	opened := openPersisted(t, dir, sessions, ResumeLatest, &scripted{})
	if opened.SessionID() != "newer-session" || !containsText(opened.Messages(), "from newer") {
		t.Fatalf("continue %s %s", opened.SessionID(), roleTexts(opened.Messages()))
	}
}

func TestSessionStampsMoveForward(t *testing.T) {
	a := nextStamp()
	b := nextStamp()
	if !b.After(a) {
		t.Fatalf("%s then %s", isoTime(a), isoTime(b))
	}
}

func TestSessionsStayInsideTheWorkspaceBucket(t *testing.T) {
	sessions := t.TempDir()
	left := openPersisted(t, t.TempDir(), sessions, "", &scripted{msgs: []ai.Message{ai.Assistant("L", ai.StopStop)}})
	right := openPersisted(t, t.TempDir(), sessions, "", &scripted{msgs: []ai.Message{ai.Assistant("R", ai.StopStop)}})
	if err := left.Prompt(context.Background(), "left-word"); err != nil {
		t.Fatal(err)
	}
	if err := right.Prompt(context.Background(), "right-word"); err != nil {
		t.Fatal(err)
	}
	again := openPersisted(t, left.Root(), sessions, ResumeLatest, &scripted{})
	if again.SessionID() != left.SessionID() || !containsText(again.Messages(), "left-word") || containsText(again.Messages(), "right-word") {
		t.Fatalf("bucket mixed %#v", again.Messages())
	}
}

func TestEncodeProject(t *testing.T) {
	if got := encodeProject("/tmp/foo/bar"); got != "--tmp-foo-bar--" {
		t.Fatalf("%s", got)
	}
	if got := encodeProject(`C:\work\app`); got != "--C--work-app--" {
		t.Fatalf("%s", got)
	}
}

func openPersisted(t *testing.T, dir, sessions, resume string, script *scripted) *Session {
	t.Helper()
	session, err := New(Config{
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		Model:         "fake",
		SessionDir:    sessions,
		Resume:        resume,
		Stream:        script.stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.closeJournal)
	return session
}

func roleTexts(msgs []ai.Message) string {
	parts := make([]string, len(msgs))
	for i, msg := range msgs {
		parts[i] = string(msg.Role) + ":" + msg.Text
	}
	return strings.Join(parts, ",")
}

func texts(msgs []ai.Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		b.WriteString(msg.Text)
		b.WriteByte('\n')
		for _, call := range msg.ToolCalls {
			b.Write(call.Arguments)
		}
	}
	return b.String()
}

func containsText(msgs []ai.Message, needle string) bool {
	return strings.Contains(texts(msgs), needle)
}

func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b[1 : len(b)-1])
}
