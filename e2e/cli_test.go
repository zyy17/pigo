package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/cli"
	"github.com/zyy17/pigo/internal/scriptapi"
)

func TestCLIOneShotRunsTheChain(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	isolateConfig(t, home, dir)
	_, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{
		{Calls: []scriptapi.Call{{
			ID: "call_write", Name: "write_file",
			Args: `{"path":"hello.txt","content":"hi\n"}`,
		}}},
		{Text: "created"},
	})
	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", srv.URL+"/v1")
	t.Setenv("OPENAI_MODEL", "test-model")

	if err := cli.Run([]string{"-w", dir, "-p", "create hello.txt"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hi\n" {
		t.Fatalf("hello.txt %q", body)
	}
}

func TestCLISkillExpandsBeforeTheModel(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	isolateConfig(t, home, dir)
	skill := filepath.Join(dir, ".pi", "skills", "greet")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: greet\ndescription: Write a greeting.\n---\n\nWrite ping.txt containing ping.\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	script, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{
		{Calls: []scriptapi.Call{{
			ID: "call_write", Name: "write_file",
			Args: `{"path":"ping.txt","content":"ping\n"}`,
		}}},
		{Text: "ok"},
	})
	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", srv.URL+"/v1")
	t.Setenv("OPENAI_MODEL", "test-model")

	if err := cli.Run([]string{"-w", dir, "-p", "/skill:greet"}); err != nil {
		t.Fatal(err)
	}
	reqs := script.Requests()
	if len(reqs) < 1 {
		t.Fatal("no request")
	}
	user := firstUser(reqs[0])
	if !strings.Contains(user, `skill "greet"`) || !strings.Contains(user, "Write ping.txt") {
		t.Fatalf("skill prompt was not expanded:\n%s", user)
	}
	got, err := os.ReadFile(filepath.Join(dir, "ping.txt"))
	if err != nil || string(got) != "ping\n" {
		t.Fatalf("ping.txt %q err %v", got, err)
	}
}

func TestCLIConfigPrecedenceReachesTheRequest(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		project  string
		dotenv   string
		envModel string
		flag     string
		want     string
	}{
		{name: "user file", user: "model: from-user\n", want: "from-user"},
		{name: "project file", user: "model: from-user\n", project: "model: from-file\n", want: "from-file"},
		{name: "dotenv", project: "model: from-file\n", dotenv: "OPENAI_MODEL=from-dotenv\n", want: "from-dotenv"},
		{name: "environment", dotenv: "OPENAI_MODEL=from-dotenv\n", envModel: "from-env", want: "from-env"},
		{name: "flag", envModel: "from-env", flag: "from-flag", want: "from-flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			home := t.TempDir()
			isolateConfig(t, home, dir)
			if tt.user != "" {
				path := filepath.Join(home, ".pigo")
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "config.yaml"), []byte(tt.user), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.project != "" {
				if err := os.WriteFile(filepath.Join(dir, "pigo.yaml"), []byte(tt.project), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.dotenv != "" {
				if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(tt.dotenv), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			script, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{{Text: "ok"}})
			t.Setenv("PIGO_PROVIDER", "openai")
			t.Setenv("OPENAI_API_KEY", "test-key")
			t.Setenv("OPENAI_BASE_URL", srv.URL+"/v1")
			t.Setenv("OPENAI_MODEL", tt.envModel)
			args := []string{"-w", dir, "-p", "say ok"}
			if tt.flag != "" {
				args = append(args, "--model", tt.flag)
			}
			if err := cli.Run(args); err != nil {
				t.Fatal(err)
			}
			reqs := script.Requests()
			if len(reqs) != 1 || reqs[0].Model != tt.want {
				t.Fatalf("model %q, requests %#v", tt.want, reqs)
			}
		})
	}
}

func isolateConfig(t *testing.T, home, dir string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(dir)
}

func firstUser(req scriptapi.Request) string {
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			return msg.Content
		}
	}
	return ""
}
