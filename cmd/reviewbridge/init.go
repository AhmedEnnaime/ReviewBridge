package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type prompter interface {
	prompt(label string) (string, error)
}

type stdinPrompter struct {
	scanner *bufio.Scanner
	out     io.Writer
}

func newStdinPrompter(in io.Reader, out io.Writer) *stdinPrompter {
	return &stdinPrompter{scanner: bufio.NewScanner(in), out: out}
}

func (p *stdinPrompter) prompt(label string) (string, error) {
	fmt.Fprintf(p.out, "%s: ", label)
	if !p.scanner.Scan() {
		if err := p.scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return strings.TrimSpace(p.scanner.Text()), nil
}

type initValidators struct {
	anthropic *http.Client
	github    *http.Client
	gitlab    *http.Client
}

func defaultInitValidators() initValidators {
	c := &http.Client{Timeout: 10 * time.Second}
	return initValidators{anthropic: c, github: c, gitlab: c}
}

func validateAnthropicKey(client *http.Client, key string) error {
	body := `{"model":"claude-sonnet-4-6","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach Anthropic API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("Anthropic API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
}

func validateGitLabToken(client *http.Client, token string) error {
	req, err := http.NewRequest("GET", "https://gitlab.com/api/v4/user", nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach GitLab API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid GitLab token")
	}
	return nil
}

func validateGitHubToken(client *http.Client, token string) error {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach GitHub API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid GitHub token")
	}
	return nil
}

type initRunner struct {
	prompter   prompter
	validators initValidators
	configPath string
	out        io.Writer
}

func (r *initRunner) run() error {
	var anthropicKey string
	for {
		key, err := r.prompter.prompt("Anthropic API key")
		if err != nil {
			return err
		}
		if key == "" {
			fmt.Fprintln(r.out, "Anthropic API key is required")
			continue
		}
		fmt.Fprintln(r.out, "Validating Anthropic API key...")
		if err := validateAnthropicKey(r.validators.anthropic, key); err != nil {
			fmt.Fprintf(r.out, "Error: %v\n", err)
			continue
		}
		anthropicKey = key
		break
	}

	var githubToken string
	for {
		token, err := r.prompter.prompt("GitHub token (optional if using GitLab, press Enter to skip)")
		if err != nil {
			return err
		}
		if token == "" {
			break
		}
		fmt.Fprintln(r.out, "Validating GitHub token...")
		if err := validateGitHubToken(r.validators.github, token); err != nil {
			fmt.Fprintf(r.out, "Error: %v\n", err)
			continue
		}
		githubToken = token
		break
	}

	var gitlabToken string
	for {
		token, err := r.prompter.prompt("GitLab token (optional if using GitHub, press Enter to skip)")
		if err != nil && err != io.EOF {
			return err
		}
		if token == "" {
			break
		}
		fmt.Fprintln(r.out, "Validating GitLab token...")
		if err := validateGitLabToken(r.validators.gitlab, token); err != nil {
			fmt.Fprintf(r.out, "Error: %v\n", err)
			continue
		}
		gitlabToken = token
		break
	}

	if githubToken == "" && gitlabToken == "" {
		return fmt.Errorf("at least one platform token (GitHub or GitLab) is required")
	}

	if err := writeConfig(r.configPath, anthropicKey, githubToken, gitlabToken); err != nil {
		return err
	}

	fmt.Fprintf(r.out, "Config saved to %s\n", r.configPath)
	return nil
}

func writeConfig(path, anthropicKey, githubToken, gitlabToken string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "anthropic_api_key: %s\n\n", anthropicKey)
	sb.WriteString("platforms:\n")
	sb.WriteString("  github:\n")
	fmt.Fprintf(&sb, "    token: %s\n", githubToken)
	sb.WriteString("    polling_interval: 60s\n")
	if gitlabToken != "" {
		sb.WriteString("  gitlab:\n")
		fmt.Fprintf(&sb, "    token: %s\n", gitlabToken)
		sb.WriteString("    url: https://gitlab.com\n")
		sb.WriteString("    polling_interval: 60s\n")
	}
	sb.WriteString("\nclaude_code:\n")
	sb.WriteString("  sessions_path: ~/.claude/projects\n\n")
	sb.WriteString("triage:\n")
	sb.WriteString("  auto_skip_style_comments: true\n")
	sb.WriteString("  min_confidence: medium\n\n")
	sb.WriteString("notifications:\n")
	sb.WriteString("  desktop: true\n")
	sb.WriteString("  terminal: true\n")

	return os.WriteFile(path, []byte(sb.String()), 0600)
}
