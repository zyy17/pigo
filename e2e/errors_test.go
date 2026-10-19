package e2e

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
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
