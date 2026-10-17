package config

import "strings"

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
	val = strings.TrimSpace(val)
	if len(val) >= 2 {
		quote := val[0]
		if (quote == '"' || quote == '\'') && val[len(val)-1] == quote {
			val = val[1 : len(val)-1]
		}
	}
	if key == "" {
		return "", "", false
	}
	return key, val, true
}
