package coding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/ai"
)

func TestSessionReportsModelAndSkills(t *testing.T) {
	dir := t.TempDir()
	user := t.TempDir()
	skill := filepath.Join(user, "greet")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: greet\ndescription: Say hello.\n---\n\nHello.\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	session, err := New(Config{
		APIKey:        "test-key",
		Model:         "test-model",
		Workspace:     dir,
		UserSkillsDir: user,
		Stream: func(context.Context, ai.Request) (<-chan ai.Event, error) {
			return nil, errors.New("unused")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.Model() != "test-model" || session.Root() != dir {
		t.Fatalf("model %s root %s", session.Model(), session.Root())
	}
	found := session.Skills()
	if len(found) != 1 || found[0].Name != "greet" {
		t.Fatalf("skills %#v", found)
	}
	session.Abort()

	read := mustTool(t, openTemp(t), "read_file")
	if _, err := read.Execute(context.Background(), []byte(`{"path":""}`)); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatal(err)
	}
	if _, err := read.Execute(context.Background(), []byte(`{"path":"missing.txt"}`)); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatal(err)
	}
}
