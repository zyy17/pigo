package cli

import (
	"errors"
	"strings"
	"unicode"

	"github.com/zyy17/pigo/internal/coding"
)

// commandText expands /skill:name into a normal user message.
// Other lines pass through. The agent loop does not learn a new command.
func commandText(session *coding.Session, line string) (string, error) {
	name, request, ok := parseSkill(line)
	if !ok {
		return line, nil
	}
	if name == "" {
		return "", errors.New("usage: /skill:name [request]")
	}
	return session.PrepareSkill(name, request)
}

func parseSkill(line string) (name, request string, ok bool) {
	const prefix = "/skill:"
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if i := strings.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		return rest[:i], strings.TrimSpace(rest[i:]), true
	}
	return rest, "", true
}
