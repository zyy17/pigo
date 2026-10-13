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

func TestSymlinkCycleDoesNotHideSiblings(t *testing.T) {
	root := t.TempDir()
	skills := filepath.Join(root, ".pi", "skills")
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, skills, filepath.Join(skills, "loop"))
	writeSkill(t, filepath.Join(skills, "ok"), "ok-skill", "still loads")

	list := Discover(root, filepath.Join(t.TempDir(), "none"))
	if len(list) != 1 || list[0].Name != "ok-skill" {
		t.Fatalf("%#v", list)
	}
}

func TestSymlinkOutsideRootIsNotWalked(t *testing.T) {
	root := t.TempDir()
	skills := filepath.Join(root, ".pi", "skills")
	if err := os.MkdirAll(skills, 0o755); err != nil {
		t.Fatal(err)
	}

	pack := t.TempDir()
	writeSkill(t, filepath.Join(pack, "nested"), "outside-child", "should not load")
	link(t, pack, filepath.Join(skills, "pack"))

	single := t.TempDir()
	writeSkill(t, single, "linked-skill", "from outside")
	writeSkill(t, filepath.Join(single, "child"), "linked-child", "should not load")
	link(t, single, filepath.Join(skills, "linked"))

	names := skillNames(t, Discover(root, filepath.Join(t.TempDir(), "none")))
	if _, ok := names["linked-skill"]; !ok {
		t.Fatal("symlink skill root was not loaded")
	}
	if _, ok := names["outside-child"]; ok {
		t.Fatal("walked a symlink target outside the skills root")
	}
	if _, ok := names["linked-child"]; ok {
		t.Fatal("walked children of a skill root")
	}
}

func TestSkillSymlinkCannotLeaveDirectory(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.md")
	writeRaw(t, secret, "---\nname: leaked\ndescription: secret\n---\npassword\n")
	dir := filepath.Join(root, ".pi", "skills", "evil")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link(t, secret, filepath.Join(dir, "SKILL.md"))

	list := Discover(root, filepath.Join(t.TempDir(), "none"))
	if len(list) != 0 {
		t.Fatalf("loaded %#v", list)
	}
}

func TestDescriptionCountsCharacters(t *testing.T) {
	root := t.TempDir()
	short := strings.Repeat("界", 400)
	long := strings.Repeat("界", maxDescription+1)
	writeSkill(t, filepath.Join(root, ".pi", "skills", "han"), "han-skill", short)
	writeSkill(t, filepath.Join(root, ".pi", "skills", "long"), "long-skill", long)

	names := skillNames(t, Discover(root, filepath.Join(t.TempDir(), "none")))
	if names["han-skill"].Description != short {
		t.Fatalf("rune description dropped: %d", len(names["han-skill"].Description))
	}
	if names["long-skill"].Description != strings.Repeat("界", maxDescription) {
		t.Fatalf("description len %d", len([]rune(names["long-skill"].Description)))
	}
}

func TestEmptyUserDirDoesNotScanWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeRaw(t, filepath.Join(root, "SKILL.md"), "---\nname: not-a-skill\ndescription: no\n---\n")
	writeSkill(t, filepath.Join(root, ".pi", "skills", "real"), "real-skill", "yes")

	list := Discover(root, " ")
	if len(list) != 1 || list[0].Name != "real-skill" {
		t.Fatalf("%#v", list)
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

func skillNames(t *testing.T, list []Skill) map[string]Skill {
	t.Helper()
	byName := map[string]Skill{}
	for _, skill := range list {
		byName[skill.Name] = skill
	}
	return byName
}

func link(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skip(err)
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
