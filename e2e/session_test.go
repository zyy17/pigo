package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/cli"
	"github.com/zyy17/pigo/internal/scriptapi"
)

func TestCLIResumeSendsPriorContext(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	isolateConfig(t, home, dir)
	script, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{
		{Text: "noted orange"},
		{Text: "still orange"},
		{Text: "from id"},
	})
	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", srv.URL+"/v1")
	t.Setenv("OPENAI_MODEL", "test-model")

	if err := cli.Run([]string{"-w", dir, "-p", "remember the word orange"}); err != nil {
		t.Fatal(err)
	}
	if err := cli.Run([]string{"-w", dir, "-c", "-p", "what was the word"}); err != nil {
		t.Fatal(err)
	}
	reqs := script.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests %d", len(reqs))
	}
	second := joinContents(reqs[1].Messages)
	if !strings.Contains(second, "remember the word orange") || !strings.Contains(second, "noted orange") {
		t.Fatalf("resume did not send prior context:\n%s", second)
	}
	if !strings.Contains(second, "what was the word") {
		t.Fatalf("new prompt missing:\n%s", second)
	}

	id := sessionID(t, home)
	if err := cli.Run([]string{"-w", dir, "--session", id[:8], "-p", "again"}); err != nil {
		t.Fatal(err)
	}
	third := joinContents(script.Requests()[2].Messages)
	if !strings.Contains(third, "remember the word orange") || !strings.Contains(third, "still orange") {
		t.Fatalf("id resume lost context:\n%s", third)
	}
}

func TestCLINoSessionIsInvisibleToContinue(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	isolateConfig(t, home, dir)
	script, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{
		{Text: "saved"},
		{Text: "ephemeral"},
		{Text: "resumed"},
	})
	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", srv.URL+"/v1")
	t.Setenv("OPENAI_MODEL", "test-model")

	if err := cli.Run([]string{"-w", dir, "-p", "remember pebble"}); err != nil {
		t.Fatal(err)
	}
	if err := cli.Run([]string{"-w", dir, "--no-session", "-p", "secret phrase"}); err != nil {
		t.Fatal(err)
	}
	if err := cli.Run([]string{"-w", dir, "-c", "-p", "what should you know"}); err != nil {
		t.Fatal(err)
	}
	reqs := script.Requests()
	if len(reqs) != 3 {
		t.Fatalf("requests %d", len(reqs))
	}
	got := joinContents(reqs[2].Messages)
	if !strings.Contains(got, "remember pebble") || !strings.Contains(got, "saved") {
		t.Fatalf("continue lost the saved turn:\n%s", got)
	}
	if strings.Contains(got, "secret phrase") || strings.Contains(got, "ephemeral") {
		t.Fatalf("ephemeral turn was saved:\n%s", got)
	}
}

func sessionID(t *testing.T, home string) string {
	t.Helper()
	root := filepath.Join(home, ".pigo", "sessions")
	var id string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		line, _, _ := strings.Cut(string(body), "\n")
		const key = `"id":"`
		i := strings.Index(line, key)
		if i < 0 {
			t.Fatalf("header %s", line)
		}
		rest := line[i+len(key):]
		id, _, _ = strings.Cut(rest, `"`)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("no session file")
	}
	return id
}

func joinContents(msgs []scriptapi.Message) string {
	var b strings.Builder
	for _, msg := range msgs {
		b.WriteString(msg.Role)
		b.WriteByte(':')
		b.WriteString(msg.Content)
		b.WriteByte('\n')
	}
	return b.String()
}
