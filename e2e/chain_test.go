package e2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/coding"
	"github.com/zyy17/pigo/internal/scriptapi"
)

func TestToolChainPairsResults(t *testing.T) {
	dir := t.TempDir()
	script, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{
		{Calls: []scriptapi.Call{{
			ID: "call_write", Name: "write_file",
			Args: `{"path":"note.txt","content":"alpha\n"}`,
		}}},
		{Calls: []scriptapi.Call{
			{ID: "call_read", Name: "read_file", Args: `{"path":"note.txt"}`},
			{ID: "call_list", Name: "list_dir", Args: `{"path":"."}`},
		}},
		{Calls: []scriptapi.Call{{
			ID: "call_edit", Name: "edit_file",
			Args: `{"path":"note.txt","old_string":"alpha","new_string":"beta"}`,
		}}},
		{Calls: []scriptapi.Call{{
			ID: "call_bash", Name: "run_command",
			Args: `{"command":"printf 'from-bash\\n' > ran.txt"}`,
		}}},
		{Text: "done"},
	})
	session := openSession(t, dir, srv.URL+"/v1", true)
	if err := session.Prompt(context.Background(), "edit the note, then run a command"); err != nil {
		t.Fatal(err)
	}

	note, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(note) != "beta\n" {
		t.Fatalf("note.txt %q", note)
	}
	ran, err := os.ReadFile(filepath.Join(dir, "ran.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ran) != "from-bash\n" {
		t.Fatalf("ran.txt %q", ran)
	}

	reqs := script.Requests()
	if len(reqs) != 5 {
		t.Fatalf("model calls %d", len(reqs))
	}
	wantTools := []string{"read_file", "write_file", "edit_file", "list_dir", "run_command"}
	for _, name := range wantTools {
		if !contains(reqs[0].Tools, name) {
			t.Fatalf("first request tools %v", reqs[0].Tools)
		}
	}
	assertPaired(t, reqs)
	if !strings.Contains(toolText(reqs[2], "call_read"), "alpha") {
		t.Fatalf("read result was not sent back:\n%s", toolText(reqs[2], "call_read"))
	}
	if !strings.Contains(toolText(reqs[2], "call_list"), "note.txt") {
		t.Fatalf("list result was not sent back:\n%s", toolText(reqs[2], "call_list"))
	}
	if !strings.Contains(toolText(reqs[4], "call_bash"), "no output") {
		t.Fatalf("command result %#v", toolText(reqs[4], "call_bash"))
	}
}

func TestAbortDuringToolAndDuringStream(t *testing.T) {
	t.Run("command", func(t *testing.T) {
		dir := t.TempDir()
		_, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{{
			Calls: []scriptapi.Call{{
				ID: "call_sleep", Name: "run_command",
				Args: `{"command":"sleep 30"}`,
			}},
		}})
		session := newSession(t, dir, srv.URL+"/v1")
		started := make(chan struct{})
		session.Subscribe(func(ev agent.Event) {
			if ev.Type == agent.ToolStart && ev.ToolName == "run_command" {
				select {
				case <-started:
				default:
					close(started)
				}
			}
		})
		errCh := make(chan error, 1)
		go func() {
			errCh <- session.Prompt(context.Background(), "sleep")
		}()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("command did not start")
		}
		session.Abort()
		select {
		case err := <-errCh:
			if !errors.Is(err, agent.ErrInterrupted) {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("abort did not finish the turn")
		}
	})

	t.Run("stream", func(t *testing.T) {
		dir := t.TempDir()
		script, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{{Hang: true}})
		session := newSession(t, dir, srv.URL+"/v1")
		errCh := make(chan error, 1)
		go func() {
			errCh <- session.Prompt(context.Background(), "wait")
		}()
		script.Wait(t)
		session.Abort()
		select {
		case err := <-errCh:
			if !errors.Is(err, agent.ErrInterrupted) {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("abort did not cancel the stream")
		}
	})
}

func newSession(t *testing.T, dir, base string) *coding.Session {
	t.Helper()
	return openSession(t, dir, base, false)
}

func openSession(t *testing.T, dir, base string, yolo bool) *coding.Session {
	t.Helper()
	session, err := coding.New(coding.Config{
		APIKey:        "test-key",
		BaseURL:       base,
		Model:         "test-model",
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		Yolo:          yolo,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func assertPaired(t *testing.T, reqs []scriptapi.Request) {
	t.Helper()
	for i, req := range reqs {
		open := map[string]bool{}
		for _, msg := range req.Messages {
			if msg.Role == "assistant" {
				open = map[string]bool{}
				for _, call := range msg.ToolCalls {
					open[call.ID] = true
				}
			}
			if msg.Role != "tool" {
				continue
			}
			if msg.ToolCallID == "" || !open[msg.ToolCallID] {
				t.Fatalf("request %d has unpaired tool id %q", i, msg.ToolCallID)
			}
		}
	}
}

func toolText(req scriptapi.Request, id string) string {
	for _, msg := range req.Messages {
		if msg.Role == "tool" && msg.ToolCallID == id {
			return msg.Content
		}
	}
	return ""
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
