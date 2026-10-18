//go:build e2e

package live

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/cli"
	"github.com/zyy17/pigo/internal/coding"
)

func TestLiveFileTools(t *testing.T) {
	dir := workspace(t)
	session := open(t, dir)
	tr := newTrace(session)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	start := time.Now()
	err := session.Prompt(ctx, "Create note.txt in this workspace. Its only line should be pigo-live-ok.")
	if err != nil {
		t.Fatalf("write: %v\n%s", err, tr.text())
	}
	err = session.Prompt(ctx, "Edit note.txt so its only line is pigo-live-edited.")
	t.Logf("file tools %s", time.Since(start).Round(time.Millisecond))
	if err != nil {
		t.Fatalf("edit: %v\n%s", err, tr.text())
	}
	body, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil || !strings.Contains(string(body), "pigo-live-edited") {
		t.Fatalf("note.txt %q err %v\n%s", body, err, tr.text())
	}
	if !tr.saw("write_file") || !tr.saw("edit_file") {
		t.Fatalf("missing file tools\n%s", tr.text())
	}
}

func TestLiveCommand(t *testing.T) {
	dir := workspace(t)
	session := open(t, dir)
	tr := newTrace(session)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	start := time.Now()
	err := session.Prompt(ctx, "In this workspace, run this bash command and do not create the file any other way: printf 'pigo-cmd-ok\\n' > cmd.txt")
	t.Logf("command %s", time.Since(start).Round(time.Millisecond))
	if err != nil {
		t.Fatalf("command: %v\n%s", err, tr.text())
	}
	if !tr.saw("run_command") {
		t.Fatalf("run_command was not called\n%s", tr.text())
	}
	body, err := os.ReadFile(filepath.Join(dir, "cmd.txt"))
	if err != nil || !strings.Contains(string(body), "pigo-cmd-ok") {
		t.Fatalf("cmd.txt %q err %v\n%s", body, err, tr.text())
	}
}

func TestLiveSkillOneShot(t *testing.T) {
	dir := workspace(t)
	skill := filepath.Join(dir, ".pi", "skills", "marker")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "---\nname: marker\ndescription: Write a marker file.\n---\n\nCreate marker.txt in the workspace. Its only line should be pigo-skill-ok.\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- cli.Run([]string{"-w", dir, "-p", "/skill:marker"})
	}()
	select {
	case err := <-done:
		t.Logf("skill %s", time.Since(start).Round(time.Millisecond))
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	body, err := os.ReadFile(filepath.Join(dir, "marker.txt"))
	if err != nil || !strings.Contains(string(body), "pigo-skill-ok") {
		t.Fatalf("marker.txt %q err %v", body, err)
	}
}

type trace struct {
	lines []string
}

func newTrace(session *coding.Session) *trace {
	tr := &trace{}
	session.Subscribe(func(ev agent.Event) {
		switch ev.Type {
		case agent.ToolStart:
			args := string(ev.Args)
			if len(args) > 180 {
				args = args[:180]
			}
			tr.lines = append(tr.lines, "tool "+ev.ToolName+" "+args)
		case agent.MessageEnd:
			if ev.Message.Role != "assistant" {
				return
			}
			text := strings.TrimSpace(ev.Message.Text)
			if len(text) > 240 {
				text = text[:240]
			}
			tr.lines = append(tr.lines, "assistant "+text)
		case agent.AgentEnd:
			if ev.Err != nil {
				tr.lines = append(tr.lines, "error "+ev.Err.Error())
			}
		}
	})
	return tr
}

func (tr *trace) saw(name string) bool {
	for _, line := range tr.lines {
		if strings.HasPrefix(line, "tool "+name+" ") || line == "tool "+name {
			return true
		}
	}
	return false
}

func (tr *trace) text() string {
	return strings.Join(tr.lines, "\n")
}

func workspace(t *testing.T) string {
	t.Helper()
	if os.Getenv("OPENAI_API_KEY") == "" || os.Getenv("OPENAI_BASE_URL") == "" {
		t.Skip("OPENAI_API_KEY or OPENAI_BASE_URL is not set")
	}
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(dir)
	return dir
}

func open(t *testing.T, dir string) *coding.Session {
	t.Helper()
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "glm-5.3"
	}
	session, err := coding.New(coding.Config{
		APIKey:        os.Getenv("OPENAI_API_KEY"),
		BaseURL:       os.Getenv("OPENAI_BASE_URL"),
		Model:         model,
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}
