package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/coding"
	"github.com/zyy17/pigo/internal/scriptapi"
)

func TestModelErrors(t *testing.T) {
	tests := []struct {
		name  string
		reply scriptapi.Reply
		want  string
	}{
		{name: "unauthorized", reply: scriptapi.Reply{Status: http.StatusUnauthorized, Body: "nope"}, want: "HTTP 401"},
		{name: "bad request", reply: scriptapi.Reply{Status: http.StatusBadRequest, Body: "bad"}, want: "HTTP 400"},
		{name: "bad chunk", reply: scriptapi.Reply{BadJSON: true}, want: "bad stream chunk"},
		{name: "dropped stream", reply: scriptapi.Reply{Drop: true, Text: "partial"}, want: "stream ended"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			_, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{tt.reply})
			session := newSession(t, dir, srv.URL+"/v1")
			err := session.Prompt(context.Background(), "go")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDroppedPartialToolCallStaysValidJSON(t *testing.T) {
	dir := t.TempDir()
	sessions := t.TempDir()
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		bodies = append(bodies, body)
		n := len(bodies)
		mu.Unlock()
		if n == 1 {
			io.WriteString(w, "data: "+mustJSON(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{
						"tool_calls": []any{map[string]any{
							"index": 0,
							"id":    "call_1",
							"function": map[string]any{
								"name":      "read_file",
								"arguments": `{"path":`,
							},
						}},
					},
				}},
			})+"\n\n")
			return
		}
		io.WriteString(w, "data: "+mustJSON(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{"content": "ok"},
				"finish_reason": "stop",
			}},
		})+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	session, err := coding.New(coding.Config{
		APIKey:        "test-key",
		BaseURL:       srv.URL + "/v1",
		Model:         "test-model",
		Workspace:     dir,
		UserSkillsDir: t.TempDir(),
		SessionDir:    sessions,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = session.Prompt(context.Background(), "read it")
	if err == nil || !strings.Contains(err.Error(), "stream ended") {
		t.Fatalf("first prompt %v", err)
	}
	if err := session.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("requests %d", len(bodies))
	}
	args := assistantArguments(t, bodies[1])
	if args != "{}" || !json.Valid([]byte(args)) {
		t.Fatalf("next request arguments %q", args)
	}

	matches, err := filepath.Glob(filepath.Join(sessions, "*", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("session files %v", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"arguments":{}`) {
		t.Fatalf("journal %s", text)
	}
	if strings.Contains(text, `{"path":`) {
		t.Fatal("journal kept the argument fragment")
	}
}

func assistantArguments(t *testing.T, body []byte) string {
	t.Helper()
	var raw struct {
		Messages []struct {
			Role      string `json:"role"`
			ToolCalls []struct {
				Function struct {
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	for _, msg := range raw.Messages {
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}
		return msg.ToolCalls[0].Function.Arguments
	}
	t.Fatal("second request has no assistant tool call")
	return ""
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestConnectionRefused(t *testing.T) {
	dir := t.TempDir()
	_, srv := scriptapi.NewServer(t, "test-key", nil)
	base := srv.URL + "/v1"
	srv.Close()
	session := newSession(t, dir, base)
	err := session.Prompt(context.Background(), "go")
	if err == nil || !strings.Contains(err.Error(), "connect") {
		t.Fatal(err)
	}
}

func TestDroppedToolCallIsPaired(t *testing.T) {
	dir := t.TempDir()
	_, srv := scriptapi.NewServer(t, "test-key", []scriptapi.Reply{{
		Drop: true,
		Calls: []scriptapi.Call{{
			ID: "call_write", Name: "write_file",
			Args: `{"path":"x.txt","content":"no"}`,
		}},
	}})
	session := newSession(t, dir, srv.URL+"/v1")
	var sawError bool
	session.Subscribe(func(ev agent.Event) {
		if ev.Type == agent.ToolEnd && ev.ToolName == "write_file" && ev.IsError {
			sawError = true
		}
	})
	err := session.Prompt(context.Background(), "write")
	if err == nil || !strings.Contains(err.Error(), "stream ended") {
		t.Fatal(err)
	}
	if !sawError {
		t.Fatal("dropped tool call was not paired with an error result")
	}
	if _, err := os.Stat(filepath.Join(dir, "x.txt")); err == nil {
		t.Fatal("write_file ran after the stream dropped")
	}
}
