// Package e2etest is a scripted OpenAI-compatible chat server for tests.
// It streams one reply per request and records the request shape.
// It does not store the Authorization header.
package e2etest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Call is one tool call the scripted model asks for.
type Call struct {
	ID   string
	Name string
	Args string
}

// Reply is the next HTTP response.
// Status, when set, is returned as that HTTP status with Body.
// Hang blocks until the request context ends.
// BadJSON writes one invalid data line.
// Drop writes the calls or text and closes before [DONE].
type Reply struct {
	Status  int
	Body    string
	Calls   []Call
	Text    string
	Hang    bool
	BadJSON bool
	Drop    bool
}

// Message is one chat message as the server saw it.
type Message struct {
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  []Call
}

// Request is one chat-completions body, without credentials.
type Request struct {
	Model    string
	Messages []Message
	Tools    []string
}

// Script serves replies in order and keeps every request.
type Script struct {
	key   string
	mu    sync.Mutex
	next  int
	steps []Reply
	reqs  []Request
	hit   chan int
}

// NewServer starts a server that expects Bearer key.
func NewServer(t *testing.T, key string, steps []Reply) (*Script, *httptest.Server) {
	t.Helper()
	s := &Script{key: key, steps: steps, hit: make(chan int, len(steps)+1)}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	return s, srv
}

// Requests returns the recorded bodies.
func (s *Script) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.reqs))
	copy(out, s.reqs)
	return out
}

// Wait reports the index of the next request that has started, after its body
// was read. Hang replies signal before they block.
func (s *Script) Wait(t *testing.T) int {
	t.Helper()
	select {
	case n := <-s.hit:
		return n
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a request")
		return -1
	}
}

func (s *Script) serve(w http.ResponseWriter, r *http.Request) {
	if s.key != "" && r.Header.Get("Authorization") != "Bearer "+s.key {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read", http.StatusBadRequest)
		return
	}
	req := decodeRequest(body)
	s.mu.Lock()
	s.reqs = append(s.reqs, req)
	i := s.next
	s.next++
	var step Reply
	if i < len(s.steps) {
		step = s.steps[i]
	} else {
		step = Reply{Status: http.StatusInternalServerError, Body: "no scripted reply"}
	}
	s.mu.Unlock()

	if step.Hang {
		s.hit <- i
		<-r.Context().Done()
		return
	}
	s.hit <- i
	if step.Status != 0 {
		http.Error(w, step.Body, step.Status)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	if step.BadJSON {
		_, _ = io.WriteString(w, "data: {not json}\n\n")
		return
	}
	if len(step.Calls) > 0 {
		writeSSE(w, toolChunk(step.Calls, !step.Drop))
	}
	if step.Text != "" {
		writeSSE(w, textChunk(step.Text, !step.Drop && len(step.Calls) == 0))
	}
	if step.Drop {
		return
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func writeSSE(w io.Writer, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = io.WriteString(w, "data: "+string(b)+"\n\n")
}

func textChunk(text string, finish bool) map[string]any {
	choice := map[string]any{
		"delta": map[string]any{"content": text},
	}
	if finish {
		choice["finish_reason"] = "stop"
	}
	return map[string]any{"choices": []any{choice}}
}

func toolChunk(calls []Call, finish bool) map[string]any {
	wire := make([]any, len(calls))
	for i, call := range calls {
		wire[i] = map[string]any{
			"index": i,
			"id":    call.ID,
			"function": map[string]any{
				"name":      call.Name,
				"arguments": call.Args,
			},
		}
	}
	choice := map[string]any{
		"delta": map[string]any{"tool_calls": wire},
	}
	if finish {
		choice["finish_reason"] = "tool_calls"
	}
	return map[string]any{"choices": []any{choice}}
}

func decodeRequest(body []byte) Request {
	var raw struct {
		Model    string `json:"model"`
		Messages []struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCallID string          `json:"tool_call_id"`
			ToolCalls  []struct {
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(body, &raw)
	out := Request{Model: raw.Model}
	for _, msg := range raw.Messages {
		item := Message{Role: msg.Role, ToolCallID: msg.ToolCallID, Content: stringContent(msg.Content)}
		for _, call := range msg.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, Call{
				ID:   call.ID,
				Name: call.Function.Name,
				Args: call.Function.Arguments,
			})
		}
		out.Messages = append(out.Messages, item)
	}
	for _, tool := range raw.Tools {
		out.Tools = append(out.Tools, tool.Function.Name)
	}
	return out
}

func stringContent(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ""
	}
	return text
}
