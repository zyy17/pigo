package cli

import (
	"fmt"
	"strings"

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
		return "", fmt.Errorf("usage: /skill:name [request]")
	}
	return session.PrepareSkill(name, request)
}

func parseSkill(line string) (name, request string, ok bool) {
	const prefix = "/skill:"
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	name, request, _ = strings.Cut(rest, " ")
	return strings.TrimSpace(name), strings.TrimSpace(request), true
}
