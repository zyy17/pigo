package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "pigo-cli-home-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	if err := os.Setenv("USERPROFILE", home); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func TestMissingAPIKey(t *testing.T) {
	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "")
	err := run([]string{"-p", "hi", "-w", t.TempDir()}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatal(err)
	}
}

func TestUnknownProvider(t *testing.T) {
	t.Setenv("PIGO_PROVIDER", "anthropic")
	t.Setenv("OPENAI_API_KEY", "test-key")
	err := run([]string{"-p", "hi", "-w", t.TempDir()}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `unknown PIGO_PROVIDER "anthropic"`) {
		t.Fatal(err)
	}
}

func TestUnknownSkill(t *testing.T) {
	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:1")
	err := run([]string{"-p", "/skill:missing", "-w", t.TempDir()}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `unknown skill "missing"`) {
		t.Fatal(err)
	}
}

func TestHelp(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-h"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "OPENAI_API_KEY") || !strings.Contains(text, "PIGO_PROVIDER") || !strings.Contains(text, "/skill:name") {
		t.Fatalf("usage:\n%s", text)
	}
	if !strings.Contains(text, "Inside the session:") {
		t.Fatalf("usage listed /skill:name as a flag:\n%s", text)
	}
}

func TestREPLCommands(t *testing.T) {
	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("OPENAI_MODEL", "test-model")
	var out bytes.Buffer
	err := run([]string{"-w", t.TempDir()}, strings.NewReader("/reset\n/exit\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "transcript cleared") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestREPLAcceptsLongLine(t *testing.T) {
	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("OPENAI_MODEL", "test-model")
	line := strings.Repeat("a", 80*1024)
	var out bytes.Buffer
	err := run([]string{"-w", t.TempDir()}, strings.NewReader(line+"\n/exit\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "token too long") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestOneShotCreatesFile(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			io.WriteString(w, sse(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{
						"tool_calls": []any{map[string]any{
							"index": 0,
							"id":    "call_w",
							"function": map[string]any{
								"name":      "write_file",
								"arguments": `{"path":"hello.txt","content":"hi"}`,
							},
						}},
					},
					"finish_reason": "tool_calls",
				}},
			}))
			return
		}
		io.WriteString(w, sse(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{"content": "created hello.txt"},
				"finish_reason": "stop",
			}},
		}))
	}))
	defer srv.Close()

	t.Setenv("PIGO_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", srv.URL+"/v1")
	t.Setenv("OPENAI_MODEL", "test-model")

	var out bytes.Buffer
	err := run([]string{"-w", dir, "-p", "create hello.txt"}, strings.NewReader(""), &out)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hi" {
		t.Fatalf("file %q", body)
	}
	text := out.String()
	if !strings.Contains(text, "write_file") || !strings.Contains(text, "created hello.txt") {
		t.Fatalf("output:\n%s", text)
	}
	if calls.Load() != 2 {
		t.Fatalf("model calls %d", calls.Load())
	}
}

func sse(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return "data: " + string(b) + "\n\ndata: [DONE]\n\n"
}
