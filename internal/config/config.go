// Package config loads pigo settings from files, .env, and the environment.
// The agent packages do not import it.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// DefaultModel is the built-in model name. The CLI help text uses this
// same constant. coding keeps its own fallback so it does not import config.
const DefaultModel = "gpt-5.6-luna"

// Config is every setting the CLI needs. Prompt comes from flags only.
type Config struct {
	Provider  string `koanf:"provider"`
	APIKey    string `koanf:"api_key"`
	BaseURL   string `koanf:"base_url"`
	Model     string `koanf:"model"`
	Workspace string `koanf:"workspace"`
	Yolo      bool   `koanf:"yolo"`
	Prompt    string `koanf:"-"`
}

// envNames maps process variables onto the flat config keys.
var envNames = map[string]string{
	"PIGO_PROVIDER":   "provider",
	"OPENAI_API_KEY":  "api_key",
	"OPENAI_BASE_URL": "base_url",
	"OPENAI_MODEL":    "model",
}

// Load merges settings. Each later source overrides the earlier one:
// defaults, ~/.pigo/config.yaml, ./pigo.yaml, .env, the environment, then flags.
// Blank values are skipped. .env also fills unset process variables so the shell tool can see them.
func Load(flags Config) (Config, error) {
	k := koanf.New(".")
	if err := loadMap(k, defaults()); err != nil {
		return Config{}, err
	}
	if err := loadFile(k, userPath()); err != nil {
		return Config{}, err
	}
	if err := loadFile(k, "pigo.yaml"); err != nil {
		return Config{}, err
	}
	// Snapshot the process environment before .env. loadDotEnv copies .env
	// into empty process variables for the shell tool. Loading this snapshot
	// afterwards is what makes a real process value win over .env.
	env := envMap()
	if err := loadDotEnv(k, ".env"); err != nil {
		return Config{}, err
	}
	if err := loadMap(k, env); err != nil {
		return Config{}, err
	}
	if err := loadMap(k, flagMap(flags)); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, err
	}
	cfg.Prompt = flags.Prompt
	return cfg.trimmed(), nil
}

func (cfg Config) trimmed() Config {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.Workspace = strings.TrimSpace(cfg.Workspace)
	cfg.Prompt = strings.TrimSpace(cfg.Prompt)
	return cfg
}

func defaults() map[string]any {
	return map[string]any{
		"provider":  "openai",
		"base_url":  "https://api.openai.com/v1",
		"model":     DefaultModel,
		"workspace": ".",
	}
}

func userPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".pigo", "config.yaml")
}

func loadFile(k *koanf.Koanf, path string) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	return nil
}

func loadMap(k *koanf.Koanf, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return k.Load(confmap.Provider(values, "."), nil)
}

func envMap() map[string]any {
	out := map[string]any{}
	for envKey, name := range envNames {
		if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
			out[name] = v
		}
	}
	if yoloOn(os.Getenv("PIGO_YOLO")) {
		out["yolo"] = true
	}
	return out
}

func yoloOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func flagMap(flags Config) map[string]any {
	out := map[string]any{}
	put := func(name, value string) {
		if v := strings.TrimSpace(value); v != "" {
			out[name] = v
		}
	}
	put("provider", flags.Provider)
	put("api_key", flags.APIKey)
	put("base_url", flags.BaseURL)
	put("model", flags.Model)
	put("workspace", flags.Workspace)
	// A false flag must not clear yolo: true from a file.
	if flags.Yolo {
		out["yolo"] = true
	}
	return out
}

func loadDotEnv(k *koanf.Koanf, path string) error {
	vals, err := readDotEnv(path)
	if err != nil || len(vals) == 0 {
		return err
	}
	overlay := map[string]any{}
	for key, val := range vals {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, val); err != nil {
				return err
			}
		}
		if name, ok := envNames[key]; ok {
			overlay[name] = val
		}
		if key == "PIGO_YOLO" && yoloOn(val) {
			overlay["yolo"] = true
		}
	}
	return loadMap(k, overlay)
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
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		key, val, ok := parseEnvLine(sc.Text())
		if ok {
			out[key] = val
		}
	}
	return out, sc.Err()
}
