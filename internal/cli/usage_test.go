package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/zyy17/pigo/internal/agent"
	"github.com/zyy17/pigo/internal/ai"
)

func TestCompactTokens(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1K"},
		{1200, "1.2K"},
		{1234, "1.23K"},
		{1500, "1.5K"},
		{10500, "10.5K"},
		{100000, "100K"},
		{1234567, "1.23M"},
	}
	for _, tc := range cases {
		if got := compactTokens(tc.n); got != tc.want {
			t.Fatalf("compactTokens(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestRenderPrintsUsage(t *testing.T) {
	var buf bytes.Buffer
	render(&buf, agent.Event{Type: agent.MessageUpdate, TextDelta: "ok"})
	render(&buf, agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("ok", ai.StopStop),
		Usage:   &ai.Usage{Input: 120, Output: 40},
		Session: &ai.Usage{Input: 1200, Output: 90},
	})
	got := buf.String()
	if got != "ok\n120 in · 40 out · session 1.2K in · 90 out\n" {
		t.Fatalf("print:\n%s", got)
	}

	buf.Reset()
	render(&buf, agent.Event{Type: agent.MessageUpdate, TextDelta: "ok"})
	render(&buf, agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("ok", ai.StopStop),
		Usage:   &ai.Usage{Input: 10, Output: 4},
		Session: &ai.Usage{Input: 10, Output: 4},
	})
	if strings.Contains(buf.String(), "session") || !strings.Contains(buf.String(), "10 in · 4 out") {
		t.Fatalf("single call:\n%s", buf.String())
	}

	buf.Reset()
	render(&buf, agent.Event{Type: agent.MessageUpdate, TextDelta: "ok"})
	render(&buf, agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("ok", ai.StopStop),
		Session: &ai.Usage{Input: 1200, Output: 90},
	})
	if strings.Contains(buf.String(), " in · ") {
		t.Fatalf("omitted call printed the session:\n%s", buf.String())
	}

	buf.Reset()
	render(&buf, agent.Event{Type: agent.MessageEnd, Message: ai.Assistant("ok", ai.StopStop)})
	if strings.Contains(buf.String(), " in · ") {
		t.Fatalf("omitted usage was printed:\n%s", buf.String())
	}
}

func TestFooterShowsUsage(t *testing.T) {
	m := testChat(t)
	m.resize(140, 24)
	if strings.Contains(ansi.Strip(m.View().Content), " in · ") {
		t.Fatalf("footer invented usage:\n%s", ansi.Strip(m.View().Content))
	}
	blank := ansi.Strip(styleFooter("ready", "glm-5.3", "/tmp/pigo", "", 140))
	if strings.Contains(blank, " in") || strings.Contains(blank, "out") {
		t.Fatalf("empty token segment rendered: %s", blank)
	}

	m.apply(agent.Event{
		Type:    agent.MessageEnd,
		Message: ai.Assistant("ok", ai.StopStop),
		Usage:   &ai.Usage{Input: 120, Output: 40},
		Session: &ai.Usage{Input: 1200, Output: 40},
	})
	got := ansi.Strip(m.View().Content)
	if !strings.Contains(got, "1.2K in") || !strings.Contains(got, "40 out") {
		t.Fatalf("footer:\n%s", got)
	}
	if strings.Contains(got, "120 in") {
		t.Fatalf("footer showed the call instead of the session:\n%s", got)
	}

	styled := styleFooter("ready", "glm-5.3", "/tmp/pigo", "1.2K in · 40 out", 140)
	plain := ansi.Strip(styled)
	if ansi.StringWidth(styled) > 140 || !strings.Contains(plain, "1.2K in") || !strings.Contains(plain, "/exit") {
		t.Fatalf("styled footer %d:\n%s", ansi.StringWidth(styled), plain)
	}

	m.apply(agent.Event{Type: agent.MessageEnd, Message: ai.User("next")})
	if !strings.Contains(ansi.Strip(m.View().Content), "1.2K in") {
		t.Fatal("a message without usage cleared the session total")
	}

	m.submit("/reset")
	got = ansi.Strip(m.View().Content)
	if strings.Contains(got, "1.2K in") || strings.Contains(got, "40 out") {
		t.Fatalf("reset left usage:\n%s", got)
	}
}
