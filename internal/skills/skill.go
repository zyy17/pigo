package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxDescription = 1024

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
// A directory symlink is not walked when its target is outside that skills root.
// Each resolved directory is visited once, so a symlink cycle stops.
// Pi also applies ignore files; pigo does not, so the walk stays easy to read.
// A missing directory is not an error. An empty directory is ignored.
func Discover(workspace, userDir string) []Skill {
	var list []Skill
	names := map[string]bool{}
	visited := map[string]bool{}
	add(userDir, &list, names, visited)
	add(ProjectDir(workspace), &list, names, visited)
	return list
}

func add(dir string, list *[]Skill, names, visited map[string]bool) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return
	}
	for _, skill := range loadDir(dir, root, true, visited) {
		if names[skill.Name] {
			continue
		}
		names[skill.Name] = true
		*list = append(*list, skill)
	}
}

// loadDir lists one directory. top is true only for the skills root, where a
// standalone .md file with frontmatter also counts as a skill.
// root is the resolved skills root being scanned. visited holds resolved paths.
func loadDir(dir, root string, top bool, visited map[string]bool) []Skill {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || visited[resolved] {
		return nil
	}
	visited[resolved] = true
	// A link to a tree outside this root must not be listed. ReadDir on that
	// target can walk far more than the skills directory. A SKILL.md at the
	// link itself is still one skill. Nested links that stay inside the root
	// are still walked.
	if !within(root, resolved) {
		return oneSkill(filepath.Join(dir, "SKILL.md"))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	if path, ok := skillMarkdown(dir, entries); ok {
		return oneSkill(path)
	}
	return walkChildren(dir, root, top, entries, visited)
}

func skillMarkdown(dir string, entries []os.DirEntry) (string, bool) {
	for _, entry := range entries {
		if entry.Name() == "SKILL.md" {
			return filepath.Join(dir, entry.Name()), true
		}
	}
	return "", false
}

func oneSkill(path string) []Skill {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() {
		return nil
	}
	skill, ok := loadFile(path)
	if !ok {
		return nil
	}
	return []Skill{skill}
}

func walkChildren(dir, root string, top bool, entries []os.DirEntry, visited map[string]bool) []Skill {
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
			list = append(list, loadDir(path, root, false, visited)...)
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

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	if description == "" {
		return Skill{}, false
	}
	abs, err := filepath.Abs(path)
	if err != nil || !fileInDir(abs) {
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
		Description:  clipRunes(description, maxDescription),
		Path:         abs,
		BaseDir:      filepath.Dir(abs),
		DisableModel: meta["disable-model-invocation"] == "true",
	}, true
}

// fileInDir rejects a SKILL.md symlink whose target leaves the skill directory.
// Following that link would read a file the skill root does not contain.
func fileInDir(path string) bool {
	realFile, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	realDir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return false
	}
	return within(realDir, realFile)
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
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
