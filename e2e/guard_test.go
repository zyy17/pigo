package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/coding"
	"github.com/zyy17/pigo/internal/e2etest"
)

func TestDangerousCommandDenied(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keep, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	script, srv := e2etest.NewServer(t, "test-key", []e2etest.Reply{
		{Calls: []e2etest.Call{{ID: "c1", Name: "run_command", Args: `{"command":"rm -rf /"}`}}},
		{Text: "stopped"},
	})
	session := newSession(t, dir, srv.URL+"/v1")
	if err := session.Prompt(context.Background(), "clean the disk"); err != nil {
		t.Fatal(err)
	}
	reqs := script.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests %d", len(reqs))
	}
	assertPaired(t, reqs)
	text := toolText(reqs[1], "c1")
	if !strings.Contains(text, "blocked") || !strings.Contains(text, "recursive delete") {
		t.Fatalf("result %q", text)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal(err)
	}
}

func TestYoloRunsMatchedCommand(t *testing.T) {
	dir := t.TempDir()
	script, srv := e2etest.NewServer(t, "test-key", []e2etest.Reply{
		{Calls: []e2etest.Call{{ID: "c1", Name: "run_command", Args: `{"command":"git push --force"}`}}},
		{Text: "done"},
	})
	session, err := coding.New(coding.Config{
		APIKey:        "test-key",
		BaseURL:       srv.URL + "/v1",
		Model:         "test-model",
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		Yolo:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Prompt(context.Background(), "publish"); err != nil {
		t.Fatal(err)
	}
	reqs := script.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests %d", len(reqs))
	}
	text := toolText(reqs[1], "c1")
	if strings.HasPrefix(text, "blocked") {
		t.Fatalf("yolo still blocked: %q", text)
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("empty tool result")
	}
}

func TestSafeDeleteStillRuns(t *testing.T) {
	dir := t.TempDir()
	build := filepath.Join(dir, "build")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, "a.o"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	script, srv := e2etest.NewServer(t, "test-key", []e2etest.Reply{
		{Calls: []e2etest.Call{{ID: "c1", Name: "run_command", Args: `{"command":"rm -rf ./build"}`}}},
		{Text: "removed"},
	})
	session := newSession(t, dir, srv.URL+"/v1")
	if err := session.Prompt(context.Background(), "drop the build"); err != nil {
		t.Fatal(err)
	}
	text := toolText(script.Requests()[1], "c1")
	if strings.Contains(text, "blocked") {
		t.Fatalf("safe command blocked: %q", text)
	}
	if _, err := os.Stat(build); !os.IsNotExist(err) {
		t.Fatalf("build still present: %v", err)
	}
}
