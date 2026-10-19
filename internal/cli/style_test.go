package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestAssistantProseUsesAccents(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"use `code` and **bold** and *note*",
		"- item",
		"see [docs](https://example.com)",
		"> aside",
		"```",
		"plain code",
		"+ added",
		"- removed",
		"```",
		"--- a/file",
		"+++ b/file",
		"- old",
		"+ new",
	}, "\n")
	got := styleProse(src)
	plain := ansi.Strip(got)
	for _, want := range []string{"# Title", "code", "bold", "note", "- item", "docs (https://example.com)", "aside", "plain code", "+ added", "- removed", "- old", "+ new"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("styled prose dropped %q:\n%s", want, plain)
		}
	}

	title := styleProse("# Title")
	body := styleProse("hello")
	code := styleProse("use `code`")
	bold := styleProse("**bold**")
	if title == body || code == body || ansiCodes(bold) == ansiCodes(body) {
		t.Fatalf("heading, code, or bold collapsed into body text:\n%q\n%q\n%q", title, code, bold)
	}
	if !strings.Contains(body, "232;217;168") {
		t.Fatalf("prose is not #E8D9A8: %q", body)
	}
	if strings.Contains(title, "232;217;168") || strings.Contains(styleProse("`code`"), "232;217;168") {
		t.Fatal("heading or inline code used the prose yellow")
	}
	if !strings.Contains(title, "00ADD8") && !strings.Contains(title, "38;2;0;173;216") {
		t.Fatalf("heading is not cyan: %q", title)
	}

	diff := strings.Split(styleProse("--- a/f\n+++ b/f\n- old\n+ new"), "\n")
	if len(diff) != 4 || ansi.Strip(diff[2]) != "- old" || ansi.Strip(diff[3]) != "+ new" || diff[2] == diff[3] {
		t.Fatalf("diff lines: %#v", diff)
	}
	list := styleProse("- item")
	if ansiCodes(list) == ansiCodes(diff[2]) {
		t.Fatal("a list marker was colored as a deletion")
	}
}

func ansiCodes(s string) string {
	return strings.ReplaceAll(s, ansi.Strip(s), "")
}

func TestToolHeaderIsBoldAndPreviewIsDim(t *testing.T) {
	m := newChat(nil, nil)
	m.resize(80, 24)
	head := m.renderBlock(block{kind: kindTool, text: "• Ran ls"})
	preview := m.renderBlock(block{kind: kindTool, text: "└ out"})
	if !strings.Contains(head, "\x1b[1m") {
		t.Fatalf("ran header is not bold: %q", head)
	}
	if strings.Contains(preview, "\x1b[1m") || !strings.Contains(preview, "\x1b[") {
		t.Fatalf("preview should be dim, not bold: %q", preview)
	}
	user := ansi.Strip(m.paintUser("hello"))
	lines := strings.Split(user, "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], "> hello") || strings.TrimSpace(lines[0]) != "" {
		t.Fatalf("user cell: %#v", lines)
	}
}
