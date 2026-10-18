package skills

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInvocationAndDefaultDir(t *testing.T) {
	dir := DefaultUserDir()
	if dir != "" && !strings.HasSuffix(dir, filepath.Join(".pi", "agent", "skills")) {
		t.Fatalf("user dir %s", dir)
	}
	got := Invocation(Skill{Name: "greet", BaseDir: filepath.Join("skills", "greet")}, "body", "hi")
	for _, want := range []string{`skill "greet"`, "skills/greet", "body\n", "User request: hi"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	if !strings.Contains(Invocation(Skill{Name: "greet", BaseDir: "dir"}, "body", ""), "Follow the skill now.") {
		t.Fatal(got)
	}
}
