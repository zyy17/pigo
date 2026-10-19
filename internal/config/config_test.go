package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PIGO_PROVIDER", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "from-env")
	t.Setenv("FROM_DOTENV", "")

	user := filepath.Join(home, ".pigo")
	if err := os.MkdirAll(user, 0o755); err != nil {
		t.Fatal(err)
	}
	userFile := "provider: openai\napi_key: user-key\nmodel: from-user\nworkspace: /user\n"
	if err := os.WriteFile(filepath.Join(user, "config.yaml"), []byte(userFile), 0o644); err != nil {
		t.Fatal(err)
	}
	project := "model: from-project\nworkspace: /project\n"
	if err := os.WriteFile(filepath.Join(dir, "pigo.yaml"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	dotenv := "export OPENAI_MODEL=\"from-dotenv\"\nOPENAI_BASE_URL=http://dotenv/v1\nOPENAI_API_KEY='secret'\nFROM_DOTENV=yes\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Config{BaseURL: "http://flag/v1", Workspace: "/flag"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "from-env" {
		t.Fatalf("model %q", cfg.Model)
	}
	if cfg.BaseURL != "http://flag/v1" || cfg.Workspace != "/flag" {
		t.Fatalf("flags did not win: %#v", cfg)
	}
	if cfg.APIKey != "secret" {
		t.Fatalf("api key %q", cfg.APIKey)
	}
	if os.Getenv("FROM_DOTENV") != "yes" {
		t.Fatalf("dotenv export %q", os.Getenv("FROM_DOTENV"))
	}
}

func TestDotEnvDoesNotOverrideEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENAI_MODEL", "already")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PIGO_PROVIDER", "")
	body := "# comment\nexport OPENAI_MODEL=\"from-file\"\nOPENAI_BASE_URL=http://example.test/v1\nOPENAI_API_KEY='secret'\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "already" {
		t.Fatalf("model overridden: %s", cfg.Model)
	}
	if cfg.BaseURL != "http://example.test/v1" || cfg.APIKey != "secret" {
		t.Fatalf("%#v", cfg)
	}
}

func TestLoadRejectsBadYAML(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PIGO_PROVIDER", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")
	if err := os.WriteFile("pigo.yaml", []byte("model: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(Config{}); err == nil || !strings.Contains(err.Error(), "pigo.yaml") {
		t.Fatal(err)
	}
}

func TestMissingFilesUseDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PIGO_PROVIDER", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")
	cfg, err := Load(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "openai" || cfg.Model != "gpt-4o-mini" || cfg.Workspace != "." {
		t.Fatalf("%#v", cfg)
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("base %q", cfg.BaseURL)
	}
	if cfg.Yolo {
		t.Fatal("yolo defaulted on")
	}
}

func TestYoloPrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PIGO_YOLO", "")
	if err := os.WriteFile(filepath.Join(dir, "pigo.yaml"), []byte("yolo: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Yolo {
		t.Fatal("yaml yolo was cleared")
	}

	t.Setenv("PIGO_YOLO", "1")
	if err := os.WriteFile(filepath.Join(dir, "pigo.yaml"), []byte("yolo: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Yolo {
		t.Fatal("PIGO_YOLO did not win")
	}

	t.Setenv("PIGO_YOLO", "")
	cfg, err = Load(Config{Yolo: true})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Yolo {
		t.Fatal("flag did not win")
	}
}
