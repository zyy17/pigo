package coding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func TestReviewAllowWritesTheEdit(t *testing.T) {
	dir := t.TempDir()
	path := writeText(t, dir, "hello.txt", "alpha\n")
	var during string
	session := scriptedSession(t, dir, false, func(_ context.Context, c Confirm) (bool, error) {
		if c.Title != "! allow this edit?" || c.Detail != "hello.txt" {
			t.Fatalf("prompt %#v", c)
		}
		if !strings.Contains(c.Body, "-alpha\n") || !strings.Contains(c.Body, "+beta\n") {
			t.Fatalf("diff %q", c.Body)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		during = string(body)
		return true, nil
	}, editCall("hello.txt", "alpha", "beta", false), ai.Assistant("yes", ai.StopStop))

	result := toolEnd(t, session, "edit")
	if during != "alpha\n" {
		t.Fatalf("wrote before allow: %q", during)
	}
	if strings.Contains(result, "blocked") || !strings.Contains(result, "updated hello.txt") {
		t.Fatalf("result %q", result)
	}
	if got := readText(t, path); got != "beta\n" {
		t.Fatalf("file %q", got)
	}
}

func TestReviewDenyLeavesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeText(t, dir, "hello.txt", "alpha\n")
	session := scriptedSession(t, dir, false, func(context.Context, Confirm) (bool, error) {
		return false, nil
	}, editCall("hello.txt", "alpha", "beta", false), ai.Assistant("no", ai.StopStop))

	result := toolEnd(t, session, "edit")
	if !strings.Contains(result, "blocked: "+reviewDenied) {
		t.Fatalf("result %q", result)
	}
	if got := readText(t, path); got != "alpha\n" {
		t.Fatalf("file %q", got)
	}
}

func TestReviewReplaceAllShowsEveryLine(t *testing.T) {
	dir := t.TempDir()
	writeText(t, dir, "hello.txt", "a\na\n")
	var body string
	session := scriptedSession(t, dir, false, func(_ context.Context, c Confirm) (bool, error) {
		body = c.Body
		return true, nil
	}, editCall("hello.txt", "a", "b", true), ai.Assistant("yes", ai.StopStop))
	if result := toolEnd(t, session, "edit"); strings.Contains(result, "blocked") {
		t.Fatalf("result %q", result)
	}
	if strings.Count(body, "-a\n") != 2 || strings.Count(body, "+b\n") != 2 {
		t.Fatalf("diff %q", body)
	}
}

func TestReviewNilConfirmDeniesWriteAndEdit(t *testing.T) {
	dir := t.TempDir()
	writeText(t, dir, "hello.txt", "alpha\n")
	session := scriptedSession(t, dir, false, nil,
		toolMsg("c1", writeToolName, map[string]any{"path": "sub/new.txt", "content": "hi\n"}),
		ai.Assistant("no", ai.StopStop),
	)
	result := toolEnd(t, session, "write")
	if !strings.Contains(result, "blocked: "+reviewNeeded) {
		t.Fatalf("result %q", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub")); !os.IsNotExist(err) {
		t.Fatalf("denied write created a directory: %v", err)
	}

	edit := scriptedSession(t, dir, false, nil,
		editCall("hello.txt", "alpha", "beta", false),
		ai.Assistant("no", ai.StopStop),
	)
	if result := toolEnd(t, edit, "edit"); !strings.Contains(result, "blocked: "+reviewNeeded) {
		t.Fatalf("edit result %q", result)
	}
	if got := readText(t, filepath.Join(dir, "hello.txt")); got != "alpha\n" {
		t.Fatalf("file %q", got)
	}
}

func TestReviewYoloSkipsTheQuestion(t *testing.T) {
	dir := t.TempDir()
	called := false
	session := scriptedSession(t, dir, true, func(context.Context, Confirm) (bool, error) {
		called = true
		return false, nil
	}, toolMsg("c1", writeToolName, map[string]any{"path": "hello.txt", "content": "hi\n"}), ai.Assistant("yes", ai.StopStop))

	result := toolEnd(t, session, "write")
	if called {
		t.Fatal("yolo asked")
	}
	if strings.HasPrefix(result, "blocked") {
		t.Fatalf("result %q", result)
	}
	if got := readText(t, filepath.Join(dir, "hello.txt")); got != "hi\n" {
		t.Fatalf("file %q", got)
	}
}

func TestReviewSkipsADoomedEdit(t *testing.T) {
	dir := t.TempDir()
	path := writeText(t, dir, "hello.txt", "alpha\n")
	called := false
	session := scriptedSession(t, dir, false, func(context.Context, Confirm) (bool, error) {
		called = true
		return false, nil
	}, editCall("hello.txt", "missing", "beta", false), ai.Assistant("no", ai.StopStop))
	result := toolEnd(t, session, "edit")
	if called {
		t.Fatal("asked about an edit that cannot apply")
	}
	if strings.Contains(result, "blocked") || !strings.Contains(result, "not found") {
		t.Fatalf("result %q", result)
	}
	if got := readText(t, path); got != "alpha\n" {
		t.Fatalf("file %q", got)
	}
}

func TestReviewSkipsIdenticalBytes(t *testing.T) {
	dir := t.TempDir()
	path := writeText(t, dir, "hello.txt", "hi\n")
	called := false
	session := scriptedSession(t, dir, false, func(context.Context, Confirm) (bool, error) {
		called = true
		return false, nil
	}, toolMsg("c1", writeToolName, map[string]any{"path": "hello.txt", "content": "hi\n"}), ai.Assistant("yes", ai.StopStop))
	result := toolEnd(t, session, "write")
	if called {
		t.Fatal("asked about identical bytes")
	}
	if strings.Contains(result, "blocked") {
		t.Fatalf("result %q", result)
	}
	if got := readText(t, path); got != "hi\n" {
		t.Fatalf("file %q", got)
	}
}

func TestReviewOverwriteAndEmptyCreate(t *testing.T) {
	dir := t.TempDir()
	writeText(t, dir, "hello.txt", "old\n")
	var overwrite string
	session := scriptedSession(t, dir, false, func(_ context.Context, c Confirm) (bool, error) {
		overwrite = c.Title + "\n" + c.Body
		return false, nil
	}, toolMsg("c1", writeToolName, map[string]any{"path": "hello.txt", "content": "new\n"}), ai.Assistant("no", ai.StopStop))
	if result := toolEnd(t, session, "write"); !strings.Contains(result, reviewDenied) {
		t.Fatalf("result %q", result)
	}
	if !strings.Contains(overwrite, "! allow this write?") || !strings.Contains(overwrite, "-old\n") || !strings.Contains(overwrite, "+new\n") {
		t.Fatalf("overwrite %q", overwrite)
	}
	if got := readText(t, filepath.Join(dir, "hello.txt")); got != "old\n" {
		t.Fatalf("file %q", got)
	}

	var created string
	empty := scriptedSession(t, dir, false, func(_ context.Context, c Confirm) (bool, error) {
		created = c.Body
		return false, nil
	}, toolMsg("c1", writeToolName, map[string]any{"path": "empty.txt", "content": ""}), ai.Assistant("no", ai.StopStop))
	_ = toolEnd(t, empty, "create")
	if !strings.Contains(created, "create empty.txt") || !strings.Contains(created, "(empty file)") {
		t.Fatalf("create %q", created)
	}
	if _, err := os.Stat(filepath.Join(dir, "empty.txt")); !os.IsNotExist(err) {
		t.Fatal("empty file was created")
	}
}

func TestReviewOutsidePathIsNotAQuestion(t *testing.T) {
	dir := t.TempDir()
	called := false
	session := scriptedSession(t, dir, false, func(context.Context, Confirm) (bool, error) {
		called = true
		return true, nil
	}, toolMsg("c1", writeToolName, map[string]any{
		"path": "/etc/pigo-should-not-exist", "content": "x",
	}), ai.Assistant("no", ai.StopStop))
	result := toolEnd(t, session, "write")
	if called || strings.Contains(result, "blocked") || !strings.Contains(result, "outside") {
		t.Fatalf("called %v result %q", called, result)
	}
}

func TestReviewBadUTF8AndHugeFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bin.txt"), []byte{0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	huge := filepath.Join(dir, "huge.txt")
	if err := os.WriteFile(huge, []byte(strings.Repeat("a", maxFileBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	var notes []string
	session := scriptedSession(t, dir, false, func(_ context.Context, c Confirm) (bool, error) {
		notes = append(notes, c.Body)
		return false, nil
	},
		toolMsg("c1", writeToolName, map[string]any{"path": "bin.txt", "content": "ok\n"}),
		toolMsg("c2", writeToolName, map[string]any{"path": "huge.txt", "content": "z\n"}),
		ai.Assistant("no", ai.StopStop),
	)
	if err := session.Prompt(context.Background(), "overwrite"); err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Fatalf("prompts %d", len(notes))
	}
	if !strings.Contains(notes[0], "not valid UTF-8") || !strings.Contains(notes[0], "+ok") {
		t.Fatalf("utf8 %q", notes[0])
	}
	if !strings.Contains(notes[1], "new content only") || !strings.Contains(notes[1], "+z") {
		t.Fatalf("huge %q", notes[1])
	}
	if got, err := os.ReadFile(filepath.Join(dir, "bin.txt")); err != nil || string(got) != "\xff" {
		t.Fatalf("bin %q err %v", got, err)
	}
	info, err := os.Stat(huge)
	if err != nil || info.Size() != maxFileBytes+1 {
		t.Fatalf("huge size %v err %v", info, err)
	}
}

func TestReviewCancelDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := writeText(t, dir, "hello.txt", "alpha\n")
	session := scriptedSession(t, dir, false, func(context.Context, Confirm) (bool, error) {
		return false, context.Canceled
	}, editCall("hello.txt", "alpha", "beta", false))
	err := session.Prompt(context.Background(), "edit")
	if !errors.Is(err, agent.ErrInterrupted) {
		t.Fatal(err)
	}
	if got := readText(t, path); got != "alpha\n" {
		t.Fatalf("file %q", got)
	}
}

func editCall(path, old, next string, all bool) ai.Message {
	return toolMsg("c1", editToolName, map[string]any{
		"path": path, "old_string": old, "new_string": next, "replace_all": all,
	})
}

func writeText(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readText(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
