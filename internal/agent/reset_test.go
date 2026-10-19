package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/agent"
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

func TestRecordErrorStopsBeforeTheModel(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{ai.Assistant("no", ai.StopStop)}}
	ag, err := agent.New(agent.Config{
		SystemPrompt: "be helpful",
		Model:        "fake",
		Stream:       script.stream,
		Record: func(ai.Message) error {
			return errors.New("disk full")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = ag.Prompt(context.Background(), "hello")
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatal(err)
	}
	if len(script.reqs) != 0 {
		t.Fatalf("model was called %d times", len(script.reqs))
	}
	if len(ag.Messages()) != 1 || ag.Messages()[0].Text != "hello" {
		t.Fatalf("transcript %#v", ag.Messages())
	}
}

func TestSetMessagesDoesNotRecord(t *testing.T) {
	script := &fakeModel{msgs: []ai.Message{ai.Assistant("again", ai.StopStop)}}
	var recorded []string
	ag, err := agent.New(agent.Config{
		SystemPrompt: "be helpful",
		Model:        "fake",
		Stream:       script.stream,
		Record: func(msg ai.Message) error {
			recorded = append(recorded, string(msg.Role)+":"+msg.Text)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ag.SetMessages([]ai.Message{ai.User("prior")})
	if len(recorded) != 0 {
		t.Fatalf("set messages recorded %#v", recorded)
	}
	if err := ag.Prompt(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	if roles(script.reqs[0].Messages) != "system,user,user" {
		t.Fatalf("request %s", roles(script.reqs[0].Messages))
	}
	if script.reqs[0].Messages[1].Text != "prior" || script.reqs[0].Messages[2].Text != "next" {
		t.Fatalf("messages %#v", script.reqs[0].Messages)
	}
	if strings.Join(recorded, ",") != "user:next,assistant:again" {
		t.Fatalf("recorded %#v", recorded)
	}
}
