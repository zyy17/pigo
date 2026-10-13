package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill is one discovered instruction file.
// DisableModel skills are omitted from the prompt and still run through /skill:name.
type Skill struct {
	Name         string
	Description  string
	Path         string
	BaseDir      string
	DisableModel bool
}

// DefaultUserDir is Pi's user skill directory, ~/.pi/agent/skills.
func DefaultUserDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pi", "agent", "skills")
}

// ProjectDir is Pi's project skill directory, <workspace>/.pi/skills.
func ProjectDir(workspace string) string {
	return filepath.Join(workspace, ".pi", "skills")
}

// Discover loads skills from the user directory first, then the project directory.
// The first skill with a given name wins, which is Pi's collision rule, so a
// user skill overrides a project skill of the same name.
//
// A directory that contains SKILL.md is a skill root: its children are not scanned.
// Other directories are scanned recursively. Dot directories and node_modules are skipped.
// Pi also applies ignore files; pigo does not, so the walk stays easy to read.
// A missing directory is not an error.
func Discover(workspace, userDir string) []Skill {
	var list []Skill
	seen := map[string]bool{}
	add(userDir, &list, seen)
	add(ProjectDir(workspace), &list, seen)
	return list
}

func add(dir string, list *[]Skill, seen map[string]bool) {
	for _, skill := range loadDir(dir, true) {
		if seen[skill.Name] {
			continue
		}
		seen[skill.Name] = true
		*list = append(*list, skill)
	}
}

// loadDir lists one directory. top is true only for the skills root, where a
// standalone .md file with frontmatter also counts as a skill.
func loadDir(dir string, top bool) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.Name() != "SKILL.md" {
			continue
		}
		skill, ok := loadFile(filepath.Join(dir, entry.Name()))
		if !ok {
			return nil
		}
		return []Skill{skill}
	}

	var list []Skill
	for _, entry := range entries {
		if skipEntry(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			list = append(list, loadDir(path, false)...)
			continue
		}
		if top && strings.HasSuffix(entry.Name(), ".md") {
			if skill, ok := loadFile(path); ok {
				list = append(list, skill)
			}
		}
	}
	return list
}

func skipEntry(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

func loadFile(path string) (Skill, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, false
	}
	meta, ok := parseFrontmatter(string(data))
	if !ok {
		return Skill{}, false
	}
	description := strings.TrimSpace(meta["description"])
	if description == "" || len(description) > 1024 {
		return Skill{}, false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Skill{}, false
	}
	name := strings.TrimSpace(meta["name"])
	if name == "" {
		name = filepath.Base(filepath.Dir(abs))
	}
	if !validName(name) {
		return Skill{}, false
	}
	return Skill{
		Name:         name,
		Description:  description,
		Path:         abs,
		BaseDir:      filepath.Dir(abs),
		DisableModel: meta["disable-model-invocation"] == "true",
	}, true
}

// Invocation is the user message for /skill:name.
// It carries the whole SKILL.md so the model does not have to decide to read it.
func Invocation(skill Skill, fileText, request string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Follow the skill %q. Its directory is %s.\n", skill.Name, filepath.ToSlash(skill.BaseDir))
	b.WriteString("Resolve relative paths in the skill against that directory.\n\n")
	b.WriteString(fileText)
	if fileText != "" && !strings.HasSuffix(fileText, "\n") {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	if strings.TrimSpace(request) == "" {
		b.WriteString("Follow the skill now.")
		return b.String()
	}
	b.WriteString("User request: ")
	b.WriteString(strings.TrimSpace(request))
	return b.String()
}
