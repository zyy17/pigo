package coding

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zyy17/pigo/internal/skills"
)

// Pi also adds docs and project-context sections. pigo has no docs tree
// or context files, so those sections are omitted. Skills are included when
// any were discovered. The remaining text is Pi's coding-agent prompt,
// with this clone's tool names.

const preamble = "You are an expert coding assistant operating inside pi, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files."

// buildSystemPrompt joins the preamble and the tagged sections the way Pi renders a system message.
func buildSystemPrompt(root string, list []skills.Skill) string {
	parts := []string{
		preamble,
		wrapSection("tools", toolSection()),
		wrapSection("rules", ruleSection()),
	}
	if section := skills.Format(list); section != "" {
		parts = append(parts, wrapSection("skills", section))
	}
	parts = append(parts, wrapSection("cwd", filepath.ToSlash(root)))
	return strings.Join(parts, "\n\n")
}

func toolSection() string {
	lines := []string{
		"- read_file: Read file contents",
		"- write_file: Create or overwrite files",
		"- edit_file: Make precise file edits with exact text replacement",
		"- list_dir: List directory contents",
		"- run_command: Execute bash commands (ls, grep, find, etc.)",
		"",
		"In addition to the tools above, you may have access to other custom tools depending on the project.",
	}
	return strings.Join(lines, "\n")
}

func ruleSection() string {
	rules := []string{
		"Use read_file to examine files instead of cat or sed.",
		"Use write_file only for new files or complete rewrites.",
		"Use edit_file for precise changes (old_string must match exactly)",
		"Keep old_string as small as possible while still being unique in the file. Do not pad with large unchanged regions.",
		"Be concise in your responses",
		"Show file paths clearly when working with files",
	}
	var b strings.Builder
	for i, rule := range rules {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "- %s", rule)
	}
	return b.String()
}

func wrapSection(name, body string) string {
	return "<" + name + ">\n" + body + "\n</" + name + ">"
}
