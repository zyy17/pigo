package skills

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Format is the skills section of the system prompt.
// Only the name, description, and path are included. The file body stays on disk
// until the model loads it with read_file, which is how Pi keeps the prompt small.
// Skills with DisableModel set are left out; /skill:name can still run them.
func Format(list []Skill) string {
	var visible []Skill
	for _, skill := range list {
		if !skill.DisableModel {
			visible = append(visible, skill)
		}
	}
	if len(visible) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("The following skills provide specialized instructions for specific tasks.\n")
	b.WriteString("Use the read_file tool to load a skill's file when the task matches its description.\n")
	b.WriteString("When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md) and use that absolute path in tool commands.\n\n")
	b.WriteString("<available_skills>\n")
	for _, skill := range visible {
		fmt.Fprintf(&b, "  <skill>\n    <name>%s</name>\n    <description>%s</description>\n    <location>%s</location>\n  </skill>\n",
			escapeXML(skill.Name),
			escapeXML(skill.Description),
			escapeXML(filepath.ToSlash(skill.Path)),
		)
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func escapeXML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}
