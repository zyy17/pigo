package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/ai"
)

func TestStreamText(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") == "Bearer test-key"
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{"content": "Hello"},
				"finish_reason": nil,
			}},
		}))
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{"content": " there"},
				"finish_reason": "stop",
			}},
		}))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	evs := mustStream(t, srv.URL+"/v1", ai.Request{
		Model:    "test-model",
		Messages: []ai.Message{ai.User("hi")},
	})
	if !sawAuth {
		t.Fatal("missing bearer token")
	}
	var text string
	var done ai.Message
	for _, ev := range evs {
		switch ev.Type {
		case ai.EventTextDelta:
			text += ev.Text
		case ai.EventDone:
			done = ev.Message
		case ai.EventError:
			t.Fatal(ev.Err)
		}
	}
	if text != "Hello there" {
		t.Fatalf("deltas %q", text)
	}
	if done.Text != "Hello there" || done.StopReason != ai.StopStop {
		t.Fatalf("done %#v", done)
	}
}

func TestStreamToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": 0,
						"id":    "call_read",
						"function": map[string]any{
							"name":      "read_file",
							"arguments": `{"path":`,
						},
					}},
				},
			}},
		}))
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{
					"tool_calls": []any{
						map[string]any{
							"index": 0,
							"function": map[string]any{
								"arguments": `"a.txt"}`,
							},
						},
						map[string]any{
							"index": 1,
							"id":    "call_list",
							"function": map[string]any{
								"name":      "list_dir",
								"arguments": `{"path":"."}`,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			}},
		}))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	evs := mustStream(t, srv.URL, ai.Request{Model: "m", Messages: []ai.Message{ai.User("go")}})
	var done ai.Message
	for _, ev := range evs {
		if ev.Type == ai.EventError {
			t.Fatal(ev.Err)
		}
		if ev.Type == ai.EventDone {
			done = ev.Message
		}
	}
	calls := done.ToolCalls
	if done.StopReason != ai.StopToolUse || len(calls) != 2 {
		t.Fatalf("message %#v", done)
	}
	if calls[0].ID != "call_read" || calls[0].Name != "read_file" || string(calls[0].Arguments) != `{"path":"a.txt"}` {
		t.Fatalf("first call %#v", calls[0])
	}
	if calls[1].Name != "list_dir" || string(calls[1].Arguments) != `{"path":"."}` {
		t.Fatalf("second call %#v", calls[1])
	}
}

func TestRequestShape(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{"content": "ok"},
				"finish_reason": "stop",
			}},
		}))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	assistant := ai.Assistant("", ai.StopToolUse, ai.ToolCall{
		ID: "call_1", Name: "read_file", Arguments: []byte(`{"path":"a.txt"}`),
	})
	result := ai.ToolResult("call_1", "read_file", "missing", true)
	evs := mustStream(t, srv.URL+"/chat/completions", ai.Request{
		Model: "demo",
		Messages: ai.WithSystem("be brief", []ai.Message{
			ai.User("read a.txt"),
			assistant,
			result,
		}),
		Tools: []ai.Tool{{
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  ai.ObjectSchema(map[string]ai.Property{"path": ai.StringProperty("path")}, "path"),
		}},
	})
	for _, ev := range evs {
		if ev.Type == ai.EventError {
			t.Fatal(ev.Err)
		}
	}

	if body["model"] != "demo" || body["stream"] != true {
		t.Fatalf("body model/stream: %#v %#v", body["model"], body["stream"])
	}
	// GPT-5 reasoning models reject max_tokens and non-default temperature or top_p.
	// Leaving them off also keeps compatible providers such as Zhipu unchanged.
	for _, key := range []string{"max_tokens", "max_completion_tokens", "temperature", "top_p"} {
		if _, ok := body[key]; ok {
			t.Fatalf("request set %s", key)
		}
	}
	msgs, _ := body["messages"].([]any)
	roles := make([]string, 0, len(msgs))
	for _, raw := range msgs {
		msg := raw.(map[string]any)
		roles = append(roles, msg["role"].(string))
	}
	if strings.Join(roles, ",") != "system,user,assistant,tool" {
		t.Fatalf("roles %v", roles)
	}
	toolMsg := msgs[3].(map[string]any)
	if toolMsg["tool_call_id"] != "call_1" || toolMsg["content"] != "error: missing" {
		t.Fatalf("tool message %#v", toolMsg)
	}
	tools := body["tools"].([]any)
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "read_file" {
		t.Fatalf("tool %#v", fn)
	}
	params := fn["parameters"].(map[string]any)
	if params["additionalProperties"] != false {
		t.Fatalf("schema %#v", params)
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := &ai.Client{BaseURL: srv.URL, APIKey: "k"}
	_, err := client.Stream(context.Background(), ai.Request{Model: "m", Messages: []ai.Message{ai.User("hi")}})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "nope") {
		t.Fatal(err)
	}
}

func TestMissingAPIKey(t *testing.T) {
	client := &ai.Client{BaseURL: "http://example.test", APIKey: " "}
	_, err := client.Stream(context.Background(), ai.Request{Model: "m"})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func mustStream(t *testing.T, base string, req ai.Request) []ai.Event {
	t.Helper()
	client := &ai.Client{BaseURL: base, APIKey: "test-key"}
	ch, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var evs []ai.Event
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

func chunk(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func writeSSE(w io.Writer, data string) {
	io.WriteString(w, "data: "+data+"\n\n")
}
