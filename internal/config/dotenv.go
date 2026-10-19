package config

import (
	"bufio"
	"errors"
	"os"
	"strings"

	"github.com/knadh/koanf/v2"
)

func loadDotEnv(k *koanf.Koanf, path string) error {
	vals, err := readDotEnv(path)
	if err != nil || len(vals) == 0 {
		return err
	}
	if err := exportDotEnv(vals); err != nil {
		return err
	}
	return loadMap(k, overlay(func(key string) string { return vals[key] }))
}

// exportDotEnv fills empty process variables so the shell tool can see .env.
// A variable that is already set stays as it is.
func exportDotEnv(vals map[string]string) error {
	for key, val := range vals {
		val = strings.TrimSpace(val)
		if val == "" || os.Getenv(key) != "" {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return err
		}
	}
	return nil
}

// overlay maps PIGO_PROVIDER, OPENAI_*, and PIGO_YOLO through get.
// Load calls it once for the process environment and once for .env.
func overlay(get func(string) string) map[string]any {
	out := map[string]any{}
	for envKey, name := range envNames {
		if v := strings.TrimSpace(get(envKey)); v != "" {
			out[name] = v
		}
	}
	if yoloOn(get("PIGO_YOLO")) {
		out["yolo"] = true
	}
	return out
}

func readDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := parseEnvLine(sc.Text())
		if ok {
			out[key] = val
		}
	}
	return out, sc.Err()
}

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
	val = unquote(strings.TrimSpace(val))
	if key == "" {
		return "", "", false
	}
	return key, val, true
}

func unquote(value string) string {
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
			return value[1 : len(value)-1]
		}
	}
	return value
}
