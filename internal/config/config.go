package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

const (
	DefaultPollingInterval = 60 * time.Second
	DefaultGitLabURL       = "https://gitlab.com"
	DefaultSessionsPath    = "~/.claude/projects"
	DefaultDBPath          = "~/.reviewbridge/reviewbridge.db"
	DefaultQueueDir        = "~/.reviewbridge/queue"
	ConfigDir              = "~/.reviewbridge"
	ConfigFile             = "config.yaml"
)

type Config struct {
	AnthropicAPIKey string        `mapstructure:"anthropic_api_key"`
	Platforms       Platforms     `mapstructure:"platforms"`
	ClaudeCode      ClaudeCode    `mapstructure:"claude_code"`
	Triage          Triage        `mapstructure:"triage"`
	Notifications   Notifications `mapstructure:"notifications"`
}

type Platforms struct {
	GitHub GitHub `mapstructure:"github"`
	GitLab GitLab `mapstructure:"gitlab"`
}

type GitHub struct {
	Token           string        `mapstructure:"token"`
	PollingInterval time.Duration `mapstructure:"polling_interval"`
	BaseURL         string        `mapstructure:"base_url"`
}

type GitLab struct {
	Token           string        `mapstructure:"token"`
	URL             string        `mapstructure:"url"`
	PollingInterval time.Duration `mapstructure:"polling_interval"`
	BaseURL         string        `mapstructure:"base_url"`
}

type ClaudeCode struct {
	SessionsPath string `mapstructure:"sessions_path"`
}

type Triage struct {
	AutoSkipStyleComments bool   `mapstructure:"auto_skip_style_comments"`
	MinConfidence         string `mapstructure:"min_confidence"`
}

type Notifications struct {
	Desktop  bool `mapstructure:"desktop"`
	Terminal bool `mapstructure:"terminal"`
}

func Load() (*Config, error) {
	return LoadFrom(expandHome(filepath.Join(ConfigDir, ConfigFile)))
}

func LoadFrom(path string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetConfigFile(path)
	v.AutomaticEnv()

	v.BindEnv("platforms.github.base_url", "REVIEWBRIDGE_GITHUB_BASE_URL") //nolint:errcheck
	v.BindEnv("platforms.gitlab.base_url", "REVIEWBRIDGE_GITLAB_BASE_URL") //nolint:errcheck

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s: run 'reviewbridge init' to create it", path)
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.AnthropicAPIKey == "" {
		return errors.New("anthropic_api_key is required")
	}
	if c.Platforms.GitHub.Token == "" && c.Platforms.GitLab.Token == "" {
		return errors.New("at least one platform token is required (github.token or gitlab.token)")
	}
	return nil
}

func ConfigPath() string {
	return expandHome(filepath.Join(ConfigDir, ConfigFile))
}

func DBPath() string {
	return expandHome(DefaultDBPath)
}

func QueueDir() string {
	return expandHome(DefaultQueueDir)
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("platforms.github.polling_interval", DefaultPollingInterval)
	v.SetDefault("platforms.gitlab.polling_interval", DefaultPollingInterval)
	v.SetDefault("platforms.gitlab.url", DefaultGitLabURL)
	v.SetDefault("claude_code.sessions_path", DefaultSessionsPath)
	v.SetDefault("triage.auto_skip_style_comments", true)
	v.SetDefault("triage.min_confidence", "medium")
	v.SetDefault("notifications.desktop", true)
	v.SetDefault("notifications.terminal", true)
}

func expandHome(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
