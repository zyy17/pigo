package agent_test

import (
	"context"
	"testing"

	"github.com/zyy17/pigo/internal/ai"
)

func TestResetDropsTheTranscript(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{
		ai.Assistant("first", ai.StopStop),
		ai.Assistant("second", ai.StopStop),
	}}
	ag := newAgent(t, script, echoTool())
	if err := ag.Prompt(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if len(ag.Messages()) == 0 {
		t.Fatal("empty transcript")
	}
	ag.Reset()
	ag.Abort()
	if len(ag.Messages()) != 0 {
		t.Fatalf("reset left %d messages", len(ag.Messages()))
	}
	if err := ag.Prompt(context.Background(), "two"); err != nil {
		t.Fatal(err)
	}
	if roles(script.reqs[1].Messages) != "system,user" {
		t.Fatalf("request after reset %s", roles(script.reqs[1].Messages))
	}
}
