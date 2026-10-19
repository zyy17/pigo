package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
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
	parsed, err := overlay(func(key string) string { return vals[key] })
	if err != nil {
		return err
	}
	return loadMap(k, parsed)
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

// overlay maps PIGO_PROVIDER, OPENAI_*, PIGO_YOLO, PIGO_SESSION_DIR, and the
// compaction budget through get. Load calls it once for the process
// environment and once for .env.
func overlay(get func(string) string) (map[string]any, error) {
	out := map[string]any{}
	for envKey, name := range envNames {
		if v := strings.TrimSpace(get(envKey)); v != "" {
			out[name] = v
		}
	}
	if yoloOn(get("PIGO_YOLO")) {
		out["yolo"] = true
	}
	if v := strings.TrimSpace(get("PIGO_COMPACT")); v != "" {
		on, ok := compactOn(v)
		if !ok {
			return nil, fmt.Errorf("PIGO_COMPACT: want 1, true, 0, or false")
		}
		out["compact"] = on
	}
	if err := putInt(out, get, "PIGO_CONTEXT_WINDOW", "context_window", true); err != nil {
		return nil, err
	}
	if err := putInt(out, get, "PIGO_RESERVE_TOKENS", "reserve_tokens", false); err != nil {
		return nil, err
	}
	if err := putInt(out, get, "PIGO_KEEP_RECENT_TOKENS", "keep_recent_tokens", false); err != nil {
		return nil, err
	}
	return out, nil
}

func compactOn(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true":
		return true, true
	case "0", "false":
		return false, true
	default:
		return false, false
	}
}

func putInt(out map[string]any, get func(string) string, envKey, name string, positive bool) error {
	v := strings.TrimSpace(get(envKey))
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || (positive && n == 0) {
		if positive {
			return fmt.Errorf("%s: want a positive integer", envKey)
		}
		return fmt.Errorf("%s: want a non-negative integer", envKey)
	}
	out[name] = n
	return nil
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
