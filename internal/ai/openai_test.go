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
	if doneEvent(t, evs).Usage != nil {
		t.Fatalf("usage invented: %#v", doneEvent(t, evs).Usage)
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
	opts, _ := body["stream_options"].(map[string]any)
	if opts["include_usage"] != true {
		t.Fatalf("stream_options %#v", body["stream_options"])
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
	if _, ok := msgs[2].(map[string]any)["content"]; ok {
		t.Fatal("tool-call assistant set content")
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

func TestPartialToolArgumentsAreJSON(t *testing.T) {
	tests := []struct {
		name string
		args []byte
		want string
	}{
		{name: "fragment", args: []byte(`{"path":`), want: "{}"},
		{name: "empty", args: nil, want: "{}"},
		{name: "blank", args: []byte(" \t"), want: "{}"},
		{name: "object", args: []byte(`{"path":"a.txt"}`), want: `{"path":"a.txt"}`},
		{name: "null", args: []byte("null"), want: "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			evs := mustStream(t, srv.URL+"/v1", ai.Request{
				Model: "demo",
				Messages: []ai.Message{
					ai.User("read"),
					ai.Assistant("", ai.StopToolUse, ai.ToolCall{
						ID: "call_1", Name: "read_file", Arguments: tt.args,
					}),
					ai.ToolResult("call_1", "read_file", "stream ended", true),
				},
			})
			for _, ev := range evs {
				if ev.Type == ai.EventError {
					t.Fatal(ev.Err)
				}
			}
			msgs, _ := body["messages"].([]any)
			assistant := msgs[1].(map[string]any)
			calls := assistant["tool_calls"].([]any)
			fn := calls[0].(map[string]any)["function"].(map[string]any)
			got, _ := fn["arguments"].(string)
			if got != tt.want || !json.Valid([]byte(got)) {
				t.Fatalf("arguments %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmptyAssistantSendsContent(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	evs := mustStream(t, srv.URL+"/v1", ai.Request{
		Model: "demo",
		Messages: []ai.Message{
			ai.User("hi"),
			ai.Assistant("", ai.StopStop),
		},
	})
	for _, ev := range evs {
		if ev.Type == ai.EventError {
			t.Fatal(ev.Err)
		}
	}
	msgs, _ := body["messages"].([]any)
	assistant := msgs[1].(map[string]any)
	if assistant["role"] != "assistant" || assistant["content"] != "" {
		t.Fatalf("assistant %#v", assistant)
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

func TestStreamNullUsageStaysUnset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{"content": "Hi"},
				"finish_reason": "stop",
			}},
			"usage": nil,
		}))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	u := doneEvent(t, mustStream(t, srv.URL, ai.Request{Model: "m", Messages: []ai.Message{ai.User("hi")}})).Usage
	if u != nil {
		t.Fatalf("null usage became %#v", u)
	}
}

func TestNullUsageDoesNotEraseEarlierCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{"content": "Hi"},
			}},
			"usage": map[string]any{
				"prompt_tokens":     2,
				"completion_tokens": 1,
				"total_tokens":      3,
			},
		}))
		writeSSE(w, chunk(map[string]any{
			"choices": []any{},
			"usage":   nil,
		}))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	u := doneEvent(t, mustStream(t, srv.URL, ai.Request{Model: "m", Messages: []ai.Message{ai.User("hi")}})).Usage
	if u == nil || u.Input != 2 || u.Output != 1 || u.Total != 3 {
		t.Fatalf("usage %#v", u)
	}
}

func TestStreamUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{"content": "Hi"},
				"finish_reason": "stop",
			}},
		}))
		// Providers often send usage after finish, with an empty choices list.
		writeSSE(w, chunk(map[string]any{
			"choices": []any{},
			"usage": map[string]any{
				"prompt_tokens":     9,
				"completion_tokens": 12,
				"total_tokens":      21,
			},
		}))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	u := doneEvent(t, mustStream(t, srv.URL, ai.Request{Model: "m", Messages: []ai.Message{ai.User("hi")}})).Usage
	if u == nil || u.Input != 9 || u.Output != 12 || u.Total != 21 {
		t.Fatalf("usage %#v", u)
	}
}

func TestStreamUsageAliasesAndLastObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta": map[string]any{"content": "a"},
			}},
			"usage": map[string]any{
				"input_tokens":  1,
				"output_tokens": 1,
				"total_tokens":  2,
			},
		}))
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"input_tokens":  5,
				"output_tokens": 3,
			},
		}))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	u := doneEvent(t, mustStream(t, srv.URL, ai.Request{Model: "m", Messages: []ai.Message{ai.User("hi")}})).Usage
	if u == nil || u.Input != 5 || u.Output != 3 || u.Total != 0 {
		t.Fatalf("usage %#v", u)
	}
}

func TestPromptTokensWinOverInputTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(w, chunk(map[string]any{
			"choices": []any{map[string]any{
				"delta":         map[string]any{"content": "a"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     4,
				"input_tokens":      9,
				"completion_tokens": 2,
				"output_tokens":     8,
				"total_tokens":      6,
			},
		}))
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	u := doneEvent(t, mustStream(t, srv.URL, ai.Request{Model: "m", Messages: []ai.Message{ai.User("hi")}})).Usage
	if u == nil || u.Input != 4 || u.Output != 2 || u.Total != 6 {
		t.Fatalf("usage %#v", u)
	}
}

func doneEvent(t *testing.T, evs []ai.Event) ai.Event {
	t.Helper()
	for _, ev := range evs {
		if ev.Type == ai.EventError {
			t.Fatal(ev.Err)
		}
		if ev.Type == ai.EventDone {
			return ev
		}
	}
	t.Fatal("no done event")
	return ai.Event{}
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
