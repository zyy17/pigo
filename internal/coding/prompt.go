package coding

import (
	"path/filepath"
	"strings"

	"github.com/zyy17/pigo/internal/skills"
)

// Pi also adds docs and project-context sections. pigo has no docs tree
// or context files, so those sections are omitted. The wording is Pi's.

const systemPrompt = `You are an expert coding assistant operating inside pi, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files.

<tools>
- read_file: Read file contents
- write_file: Create or overwrite files
- edit_file: Make precise file edits with exact text replacement
- list_dir: List directory contents
- run_command: Execute bash commands (ls, grep, find, etc.)

In addition to the tools above, you may have access to other custom tools depending on the project.
</tools>

<rules>
- Use read_file to examine files instead of cat or sed.
- Use write_file only for new files or complete rewrites.
- Use edit_file for precise changes (old_string must match exactly)
- Keep old_string as small as possible while still being unique in the file. Do not pad with large unchanged regions.
- Be concise in your responses
- Show file paths clearly when working with files
</rules>`

// buildSystemPrompt appends the skill list, when there is one, and the working directory.
func buildSystemPrompt(root string, list []skills.Skill) string {
	var b strings.Builder
	b.WriteString(systemPrompt)
	if section := skills.Format(list); section != "" {
		b.WriteString("\n\n")
		b.WriteString(wrapSection("skills", section))
	}
	b.WriteString("\n\n")
	b.WriteString(wrapSection("cwd", filepath.ToSlash(root)))
	return b.String()
}

func wrapSection(name, body string) string {
	return "<" + name + ">\n" + body + "\n</" + name + ">"
}
