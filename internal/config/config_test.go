package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

func TestConfigLoadsFromFile(t *testing.T) {
	path := writeConfig(t, `
anthropic_api_key: sk-ant-test
platforms:
  github:
    token: ghp_test
    polling_interval: 30s
  gitlab:
    token: glpat_test
    url: https://gitlab.mycompany.com
triage:
  auto_skip_style_comments: false
notifications:
  desktop: false
  terminal: true
`)
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AnthropicAPIKey != "sk-ant-test" {
		t.Errorf("AnthropicAPIKey = %q, want %q", cfg.AnthropicAPIKey, "sk-ant-test")
	}
	if cfg.Platforms.GitHub.Token != "ghp_test" {
		t.Errorf("GitHub.Token = %q, want %q", cfg.Platforms.GitHub.Token, "ghp_test")
	}
	if cfg.Platforms.GitHub.PollingInterval != 30*time.Second {
		t.Errorf("GitHub.PollingInterval = %v, want 30s", cfg.Platforms.GitHub.PollingInterval)
	}
	if cfg.Platforms.GitLab.Token != "glpat_test" {
		t.Errorf("GitLab.Token = %q, want %q", cfg.Platforms.GitLab.Token, "glpat_test")
	}
	if cfg.Platforms.GitLab.URL != "https://gitlab.mycompany.com" {
		t.Errorf("GitLab.URL = %q, want %q", cfg.Platforms.GitLab.URL, "https://gitlab.mycompany.com")
	}
	if cfg.Triage.AutoSkipStyleComments != false {
		t.Error("Triage.AutoSkipStyleComments = true, want false")
	}
	if cfg.Notifications.Desktop != false {
		t.Error("Notifications.Desktop = true, want false")
	}
	if cfg.Notifications.Terminal != true {
		t.Error("Notifications.Terminal = false, want true")
	}
}

func TestConfigMissingFile(t *testing.T) {
	_, err := config.LoadFrom("/tmp/does-not-exist/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestConfigMissingAPIKey(t *testing.T) {
	path := writeConfig(t, `
platforms:
  github:
    token: ghp_test
`)
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing anthropic_api_key, got nil")
	}
}

func TestConfigValidateRequiresPlatformToken(t *testing.T) {
	path := writeConfig(t, `
anthropic_api_key: sk-ant-test
`)
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error when no platform tokens set, got nil")
	}
}

func TestConfigValidPassesWithGitHubOnly(t *testing.T) {
	path := writeConfig(t, `
anthropic_api_key: sk-ant-test
platforms:
  github:
    token: ghp_test
`)
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestConfigDefaultPollingInterval(t *testing.T) {
	path := writeConfig(t, `
anthropic_api_key: sk-ant-test
platforms:
  github:
    token: ghp_test
`)
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Platforms.GitHub.PollingInterval != config.DefaultPollingInterval {
		t.Errorf("PollingInterval = %v, want %v", cfg.Platforms.GitHub.PollingInterval, config.DefaultPollingInterval)
	}
	if cfg.Platforms.GitLab.PollingInterval != config.DefaultPollingInterval {
		t.Errorf("GitLab.PollingInterval = %v, want %v", cfg.Platforms.GitLab.PollingInterval, config.DefaultPollingInterval)
	}
}

func TestConfigGitLabURLDefaults(t *testing.T) {
	path := writeConfig(t, `
anthropic_api_key: sk-ant-test
platforms:
  github:
    token: ghp_test
`)
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Platforms.GitLab.URL != config.DefaultGitLabURL {
		t.Errorf("GitLab.URL = %q, want %q", cfg.Platforms.GitLab.URL, config.DefaultGitLabURL)
	}
}

func TestConfigBaseURLOverrideViaEnv(t *testing.T) {
	t.Setenv("REVIEWBRIDGE_GITHUB_BASE_URL", "http://localhost:8080")
	t.Setenv("REVIEWBRIDGE_GITLAB_BASE_URL", "http://localhost:8081")

	path := writeConfig(t, `
anthropic_api_key: sk-ant-test
platforms:
  github:
    token: ghp_test
`)
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Platforms.GitHub.BaseURL != "http://localhost:8080" {
		t.Errorf("GitHub.BaseURL = %q, want http://localhost:8080", cfg.Platforms.GitHub.BaseURL)
	}
	if cfg.Platforms.GitLab.BaseURL != "http://localhost:8081" {
		t.Errorf("GitLab.BaseURL = %q, want http://localhost:8081", cfg.Platforms.GitLab.BaseURL)
	}
}
