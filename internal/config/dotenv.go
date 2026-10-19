package config

import (
	"strings"

	"github.com/zyy17/pigo/internal/skills"
)

func parseEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, val, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	val = skills.Unquote(strings.TrimSpace(val))
	if key == "" {
		return "", "", false
	}
	return key, val, true
}
