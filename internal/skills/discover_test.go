package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverRules(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(t.TempDir(), "skills")
	writeSkill(t, filepath.Join(user, "shared"), "shared", "from the user")
	writeSkill(t, filepath.Join(root, ".pi", "skills", "shared"), "shared", "from the project")
	writeSkill(t, filepath.Join(root, ".pi", "skills", "nested", "child"), "child-skill", "a nested skill")
	writeSkill(t, filepath.Join(root, ".pi", "skills", "parent"), "parent-skill", "stops the walk")
	writeRaw(t, filepath.Join(root, ".pi", "skills", "parent", "nested", "SKILL.md"), "---\nname: hidden-child\ndescription: should not load\n---\n")
	writeRaw(t, filepath.Join(root, ".pi", "skills", "note.md"), "---\nname: standalone\ndescription: a root markdown skill\n---\n\nbody\n")
	writeRaw(t, filepath.Join(root, ".pi", "skills", "broken", "SKILL.md"), "no frontmatter\n")
	writeSkill(t, filepath.Join(root, ".pi", "skills", "secret"), "secret-skill", "only through the command")
	rewriteDisable(t, filepath.Join(root, ".pi", "skills", "secret", "SKILL.md"))

	list := Discover(root, user)
	byName := map[string]Skill{}
	for _, skill := range list {
		byName[skill.Name] = skill
	}
	if byName["shared"].Description != "from the user" {
		t.Fatalf("user skill should win, got %#v", byName["shared"])
	}
	if _, ok := byName["child-skill"]; !ok {
		t.Fatal("nested skill was not found")
	}
	if _, ok := byName["hidden-child"]; ok {
		t.Fatal("a child of a skill root was loaded")
	}
	if _, ok := byName["standalone"]; !ok {
		t.Fatal("root markdown skill was not found")
	}
	if _, ok := byName["broken"]; ok {
		t.Fatal("skill without frontmatter was loaded")
	}
	if !byName["secret-skill"].DisableModel {
		t.Fatal("disable-model-invocation was not parsed")
	}
}

func TestFormatOmitsBodyAndHiddenSkills(t *testing.T) {
	list := []Skill{
		{Name: "explain-go", Description: "Explain Go & code", Path: "/tmp/explain/SKILL.md"},
		{Name: "secret", Description: "hidden", Path: "/tmp/secret/SKILL.md", DisableModel: true},
	}
	got := Format(list)
	if strings.Contains(got, "hidden") || strings.Contains(got, "# Explain") {
		t.Fatalf("prompt leaked a hidden skill or a body:\n%s", got)
	}
	for _, part := range []string{"<name>explain-go</name>", "Explain Go &amp; code", "/tmp/explain/SKILL.md", "read_file"} {
		if !strings.Contains(got, part) {
			t.Fatalf("prompt missing %q:\n%s", part, got)
		}
	}
}

func TestFrontmatterQuotes(t *testing.T) {
	dir := t.TempDir()
	writeRaw(t, filepath.Join(dir, "SKILL.md"), "---\nname: quoted-name\ndescription: \"say hello\"\n---\n\nDo the thing.\n")
	skill, ok := loadFile(filepath.Join(dir, "SKILL.md"))
	if !ok || skill.Description != "say hello" || skill.Name != "quoted-name" {
		t.Fatalf("parsed %#v ok %v", skill, ok)
	}
}

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n# " + name + "\n"
	writeRaw(t, filepath.Join(dir, "SKILL.md"), body)
}

func writeRaw(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rewriteDisable(t *testing.T, path string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	next := strings.Replace(string(body), "---\n", "---\ndisable-model-invocation: true\n", 1)
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		t.Fatal(err)
	}
}
