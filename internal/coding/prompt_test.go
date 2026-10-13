package coding

import (
	"strings"
	"testing"

	"github.com/zyy17/pigo/internal/skills"
)

func TestSystemPromptFollowsPi(t *testing.T) {
	got := buildSystemPrompt("/work/app", nil)
	want := []string{
		"You are an expert coding assistant operating inside pi, a coding agent harness.",
		"<tools>",
		"- read_file: Read file contents",
		"- edit_file: Make precise file edits with exact text replacement",
		"- run_command: Execute bash commands (ls, grep, find, etc.)",
		"In addition to the tools above, you may have access to other custom tools depending on the project.",
		"</tools>",
		"<rules>",
		"- Use read_file to examine files instead of cat or sed.",
		"- Use edit_file for precise changes (old_string must match exactly)",
		"- Be concise in your responses",
		"- Show file paths clearly when working with files",
		"</rules>",
		"<cwd>\n/work/app\n</cwd>",
	}
	for _, part := range want {
		if !strings.Contains(got, part) {
			t.Fatalf("prompt missing %q:\n%s", part, got)
		}
	}
	if strings.Contains(got, "<skills>") {
		t.Fatal("empty skill list should not add a skills section")
	}

	withSkill := buildSystemPrompt("/work/app", []skills.Skill{{
		Name:        "explain-go",
		Description: "Explain Go code",
		Path:        "/work/app/.pi/skills/explain-go/SKILL.md",
	}})
	if !strings.Contains(withSkill, "<name>explain-go</name>") || !strings.Contains(withSkill, "<skills>") {
		t.Fatalf("skill was not injected:\n%s", withSkill)
	}
}
