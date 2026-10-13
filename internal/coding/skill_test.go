package coding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func TestSkillPromptAndRead(t *testing.T) {
	workspace := t.TempDir()
	user := t.TempDir()
	skillDir := filepath.Join(user, "explain-go")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	body := "---\nname: explain-go\ndescription: Explain a Go file\n---\n\nRead the file, then explain it.\n"
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	script := &scripted{msgs: []ai.Message{
		toolMsg("call_r", "read_file", map[string]any{"path": skillPath}),
		ai.Assistant("explained", ai.StopStop),
	}}
	session, err := New(Config{
		Workspace:     workspace,
		UserSkillsDir: user,
		Model:         "fake",
		Stream:        script.stream,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "what does main do?"); err != nil {
		t.Fatal(err)
	}
	prompt := script.requests()[0].Messages[0].Text
	if !strings.Contains(prompt, "<name>explain-go</name>") || !strings.Contains(prompt, filepath.ToSlash(skillPath)) {
		t.Fatalf("system prompt did not list the skill:\n%s", prompt)
	}
	if strings.Contains(prompt, "Read the file, then explain it.") {
		t.Fatal("system prompt included the skill body")
	}

	ws := openAt(t, workspace)
	dirs := skillDirs(session.skills)
	read := toolNamed(t, ws, dirs, "read_file")
	got, err := read.Execute(context.Background(), jsonArg(t, map[string]string{"path": skillPath}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "explain it") {
		t.Fatalf("read %q", got)
	}
	write := toolNamed(t, ws, dirs, "write_file")
	if _, err := write.Execute(context.Background(), jsonArg(t, map[string]string{
		"path": filepath.Join(skillDir, "notes.txt"), "content": "nope",
	})); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("write to a skill dir: %v", err)
	}

	forced, err := session.PrepareSkill("explain-go", "start with main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(forced, "Read the file, then explain it.") || !strings.Contains(forced, "User request: start with main.go") {
		t.Fatalf("invocation %q", forced)
	}
}

func openAt(t *testing.T, dir string) *Workspace {
	t.Helper()
	ws, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func toolNamed(t *testing.T, ws *Workspace, dirs []string, name string) agent.Tool {
	t.Helper()
	for _, tool := range Tools(ws, dirs) {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return agent.Tool{}
}
