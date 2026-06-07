package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockPrompter struct {
	responses []string
	idx       int
}

func (m *mockPrompter) prompt(_ string) (string, error) {
	if m.idx >= len(m.responses) {
		return "", nil
	}
	r := m.responses[m.idx]
	m.idx++
	return r, nil
}

func newOKServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestInitWritesConfigFile(t *testing.T) {
	anthropicSrv := newOKServer(t)
	githubSrv := newOKServer(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	runner := &initRunner{
		prompter: &mockPrompter{responses: []string{"sk-ant-key", "ghp_token", ""}},
		validators: initValidators{
			anthropic: &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			github:    &http.Client{Transport: redirectTo(githubSrv.URL)},
			gitlab:    &http.Client{Transport: redirectTo(githubSrv.URL)},
		},
		configPath: cfgPath,
		out:        &strings.Builder{},
	}

	if err := runner.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if !strings.Contains(content, "sk-ant-key") {
		t.Error("config missing anthropic key")
	}
	if !strings.Contains(content, "ghp_token") {
		t.Error("config missing github token")
	}
}

func TestInitValidatesAnthropicKey(t *testing.T) {
	calls := 0
	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer anthropicSrv.Close()

	githubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer githubSrv.Close()

	dir := t.TempDir()
	var out strings.Builder

	runner := &initRunner{
		prompter: &mockPrompter{responses: []string{"bad-key", "good-key", "ghp_token", ""}},
		validators: initValidators{
			anthropic: &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			github:    &http.Client{Transport: redirectTo(githubSrv.URL)},
			gitlab:    &http.Client{Transport: redirectTo(githubSrv.URL)},
		},
		configPath: filepath.Join(dir, "config.yaml"),
		out:        &out,
	}

	if err := runner.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(out.String(), "Error:") {
		t.Error("expected error message for invalid key")
	}
	if calls < 2 {
		t.Error("expected Anthropic API to be called at least twice (retry after failure)")
	}
}

func TestInitValidatesGitHubToken(t *testing.T) {
	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer anthropicSrv.Close()

	githubCalls := 0
	githubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		githubCalls++
		if githubCalls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer githubSrv.Close()

	dir := t.TempDir()
	var out strings.Builder

	runner := &initRunner{
		prompter: &mockPrompter{responses: []string{"sk-ant-key", "bad-token", "good-token", ""}},
		validators: initValidators{
			anthropic: &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			github:    &http.Client{Transport: redirectTo(githubSrv.URL)},
			gitlab:    &http.Client{Transport: redirectTo(githubSrv.URL)},
		},
		configPath: filepath.Join(dir, "config.yaml"),
		out:        &out,
	}

	if err := runner.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(out.String(), "Error:") {
		t.Error("expected error message for invalid GitHub token")
	}
}

func TestInitSkipsGitLabIfEmpty(t *testing.T) {
	anthropicSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer anthropicSrv.Close()

	githubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer githubSrv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	runner := &initRunner{
		prompter: &mockPrompter{responses: []string{"sk-ant-key", "ghp_token", ""}},
		validators: initValidators{
			anthropic: &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			github:    &http.Client{Transport: redirectTo(githubSrv.URL)},
			gitlab:    &http.Client{Transport: redirectTo(githubSrv.URL)},
		},
		configPath: cfgPath,
		out:        &strings.Builder{},
	}

	if err := runner.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(data), "gitlab:") {
		t.Error("config should not contain GitLab section when token is empty")
	}
}

func TestInitValidatesGitLabToken(t *testing.T) {
	anthropicSrv := newOKServer(t)
	githubSrv := newOKServer(t)

	gitlabCalls := 0
	gitlabSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitlabCalls++
		if gitlabCalls == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer gitlabSrv.Close()

	dir := t.TempDir()
	var out strings.Builder

	runner := &initRunner{
		prompter: &mockPrompter{responses: []string{"sk-ant-key", "ghp_token", "bad-gitlab", "good-gitlab"}},
		validators: initValidators{
			anthropic: &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			github:    &http.Client{Transport: redirectTo(githubSrv.URL)},
			gitlab:    &http.Client{Transport: redirectTo(gitlabSrv.URL)},
		},
		configPath: filepath.Join(dir, "config.yaml"),
		out:        &out,
	}

	if err := runner.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(out.String(), "Error:") {
		t.Error("expected error message for invalid GitLab token")
	}
	if gitlabCalls < 2 {
		t.Error("expected GitLab API to be called at least twice (retry after failure)")
	}
}

func TestInitGitLabOnly(t *testing.T) {
	anthropicSrv := newOKServer(t)
	gitlabSrv := newOKServer(t)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	runner := &initRunner{
		prompter: &mockPrompter{responses: []string{"sk-ant-key", "", "glpat_token"}},
		validators: initValidators{
			anthropic: &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			github:    &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			gitlab:    &http.Client{Transport: redirectTo(gitlabSrv.URL)},
		},
		configPath: cfgPath,
		out:        &strings.Builder{},
	}

	if err := runner.run(); err != nil {
		t.Fatalf("GitLab-only init should succeed: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	content := string(data)
	if !strings.Contains(content, "glpat_token") {
		t.Error("config missing GitLab token")
	}
}

func TestInitRequiresAtLeastOnePlatformToken(t *testing.T) {
	anthropicSrv := newOKServer(t)

	runner := &initRunner{
		prompter: &mockPrompter{responses: []string{"sk-ant-key", "", ""}},
		validators: initValidators{
			anthropic: &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			github:    &http.Client{Transport: redirectTo(anthropicSrv.URL)},
			gitlab:    &http.Client{Transport: redirectTo(anthropicSrv.URL)},
		},
		configPath: filepath.Join(t.TempDir(), "config.yaml"),
		out:        &strings.Builder{},
	}

	if err := runner.run(); err == nil {
		t.Error("expected error when no platform token provided")
	}
}

type redirectTransport struct{ base string }

func redirectTo(base string) http.RoundTripper { return &redirectTransport{base: base} }

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = "http"
	req2.URL.Host = strings.TrimPrefix(rt.base, "http://")
	return http.DefaultTransport.RoundTrip(req2)
}
