package cli

import "testing"

func TestParseSkillSplitsOnNewline(t *testing.T) {
	name, request, ok := parseSkill("/skill:go-tests\nrun them")
	if !ok || name != "go-tests" || request != "run them" {
		t.Fatalf("newline name %q request %q ok %v", name, request, ok)
	}

	name, request, ok = parseSkill("/skill: go-tests run them")
	if !ok || name != "go-tests" || request != "run them" {
		t.Fatalf("space name %q request %q ok %v", name, request, ok)
	}

	name, request, ok = parseSkill("/skill:go-tests")
	if !ok || name != "go-tests" || request != "" {
		t.Fatalf("bare name %q request %q ok %v", name, request, ok)
	}
}
