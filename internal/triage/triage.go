package triage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

const (
	defaultEndpoint = "https://api.anthropic.com/v1/messages"
	defaultModel    = "claude-sonnet-4-6"
	anthropicVersion = "2023-06-01"
)

type TriageResult struct {
	CommentID string
	Verdict   string
	Reason    string
}

type Engine struct {
	apiKey   string
	model    string
	endpoint string
	db       *db.DB
	http     *http.Client
}

func New(apiKey string, d *db.DB) *Engine {
	return &Engine{
		apiKey:   apiKey,
		model:    defaultModel,
		endpoint: defaultEndpoint,
		db:       d,
		http:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *Engine) WithEndpoint(url string) *Engine {
	e.endpoint = url
	return e
}

func (e *Engine) Run(comments []*db.Comment, diff, repoPath string) ([]TriageResult, error) {
	if len(comments) == 0 {
		return nil, nil
	}

	prompt := BuildPrompt(diff, comments, repoPath)
	text, err := e.callClaude(prompt)
	if err != nil {
		return nil, err
	}

	results, err := parseResponse(text)
	if err != nil {
		return nil, err
	}

	resultMap := make(map[string]TriageResult, len(results))
	for _, r := range results {
		resultMap[r.CommentID] = r
	}

	for _, c := range comments {
		r, ok := resultMap[c.CommentID]
		if !ok {
			r = TriageResult{CommentID: c.CommentID, Verdict: db.VerdictYourCall, Reason: "not evaluated by triage"}
		}
		if err := e.db.SetTriageResult(c.CommentID, r.Verdict); err != nil {
			return nil, fmt.Errorf("set triage result for %s: %w", c.CommentID, err)
		}
	}

	return results, nil
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (e *Engine) callClaude(prompt string) (string, error) {
	body, err := json.Marshal(claudeRequest{
		Model:     e.model,
		MaxTokens: 1024,
		System:    SystemPrompt(),
		Messages:  []claudeMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", e.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := e.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude API error %d: %s", resp.StatusCode, string(raw))
	}

	var cr claudeResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("decode claude response: %w", err)
	}
	if len(cr.Content) == 0 {
		return "", fmt.Errorf("empty response from claude")
	}
	return cr.Content[0].Text, nil
}

type rawVerdict struct {
	CommentID string `json:"comment_id"`
	Verdict   string `json:"verdict"`
	Reason    string `json:"reason"`
}

func parseResponse(text string) ([]TriageResult, error) {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON array found in response")
	}

	var raw []rawVerdict
	if err := json.Unmarshal([]byte(text[start:end+1]), &raw); err != nil {
		return nil, fmt.Errorf("parse verdicts: %w", err)
	}

	results := make([]TriageResult, 0, len(raw))
	for _, r := range raw {
		if r.CommentID == "" {
			return nil, fmt.Errorf("verdict missing comment_id")
		}
		if !isValidVerdict(r.Verdict) {
			r.Verdict = db.VerdictYourCall
		}
		results = append(results, TriageResult{
			CommentID: r.CommentID,
			Verdict:   r.Verdict,
			Reason:    r.Reason,
		})
	}
	return results, nil
}

func isValidVerdict(v string) bool {
	return v == db.VerdictFix || v == db.VerdictYourCall || v == db.VerdictSkip
}
