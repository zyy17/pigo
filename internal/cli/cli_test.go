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

func TestMissingAPIKey(t *testing.T) {
	t.Setenv("PIGO_API_KEY", "")
	err := run([]string{"-p", "hi", "-w", t.TempDir()}, strings.NewReader(""), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "PIGO_API_KEY") {
		t.Fatal(err)
	}
}

func TestHelp(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-h"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "PIGO_API_KEY") || !strings.Contains(out.String(), "-p") {
		t.Fatalf("usage:\n%s", out.String())
	}
}

func TestREPLCommands(t *testing.T) {
	t.Setenv("PIGO_API_KEY", "test-key")
	t.Setenv("PIGO_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("PIGO_MODEL", "test-model")
	var out bytes.Buffer
	err := run([]string{"-w", t.TempDir()}, strings.NewReader("/reset\n/exit\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "transcript cleared") {
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

	t.Setenv("PIGO_API_KEY", "test-key")
	t.Setenv("PIGO_BASE_URL", srv.URL+"/v1")
	t.Setenv("PIGO_MODEL", "test-model")

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

func TestDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.env")
	body := "# comment\nexport PIGO_MODEL=\"from-file\"\nPIGO_BASE_URL=http://example.test/v1\nPIGO_API_KEY='secret'\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIGO_MODEL", "already")
	t.Setenv("PIGO_BASE_URL", "")
	t.Setenv("PIGO_API_KEY", "")
	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("PIGO_MODEL") != "already" {
		t.Fatalf("model overridden: %s", os.Getenv("PIGO_MODEL"))
	}
	if os.Getenv("PIGO_BASE_URL") != "http://example.test/v1" {
		t.Fatalf("base url %s", os.Getenv("PIGO_BASE_URL"))
	}
	if os.Getenv("PIGO_API_KEY") != "secret" {
		t.Fatalf("api key %q", os.Getenv("PIGO_API_KEY"))
	}
}

func sse(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return "data: " + string(b) + "\n\ndata: [DONE]\n\n"
}
