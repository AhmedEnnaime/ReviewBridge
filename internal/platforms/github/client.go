package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/platforms"
)

const defaultBaseURL = "https://api.github.com"

type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

func New(token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		token:   token,
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

type ghPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Head   struct {
		Ref string `json:"ref"`
	} `json:"head"`
	HTMLURL string `json:"html_url"`
}

type ghComment struct {
	ID   int64  `json:"id"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Body      string `json:"body"`
	Path      string `json:"path"`
	Line      *int   `json:"line"`
	CreatedAt string `json:"created_at"`
}

func (c *Client) ListOpenPullRequests(repo string) ([]*platforms.PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/pulls?state=open&per_page=100", repo)
	var out []*platforms.PullRequest
	nextURL := c.baseURL + path
	for nextURL != "" {
		body, resp, err := c.get(nextURL)
		if err != nil {
			return nil, err
		}
		var prs []ghPR
		if err := json.Unmarshal(body, &prs); err != nil {
			return nil, fmt.Errorf("decode PRs: %w", err)
		}
		for _, pr := range prs {
			out = append(out, &platforms.PullRequest{
				Number:       pr.Number,
				Title:        pr.Title,
				SourceBranch: pr.Head.Ref,
				State:        pr.State,
				HTMLURL:      pr.HTMLURL,
			})
		}
		nextURL = parseNextLink(resp.Header.Get("Link"))
	}
	return out, nil
}

func (c *Client) GetPullRequest(repo string, prID int) (*platforms.PullRequest, error) {
	path := fmt.Sprintf("/repos/%s/pulls/%d", repo, prID)
	body, _, err := c.get(c.baseURL + path)
	if err != nil {
		return nil, err
	}
	var pr ghPR
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("decode PR: %w", err)
	}
	return &platforms.PullRequest{
		Number:       pr.Number,
		Title:        pr.Title,
		SourceBranch: pr.Head.Ref,
		State:        pr.State,
		HTMLURL:      pr.HTMLURL,
	}, nil
}

func (c *Client) ListCommentsSince(repo string, prID int, since time.Time) ([]*platforms.Comment, error) {
	sinceStr := url.QueryEscape(since.UTC().Format(time.RFC3339))
	path := fmt.Sprintf("/repos/%s/pulls/%d/comments?since=%s&per_page=100", repo, prID, sinceStr)
	var out []*platforms.Comment
	nextURL := c.baseURL + path
	for nextURL != "" {
		body, resp, err := c.get(nextURL)
		if err != nil {
			return nil, err
		}
		var comments []ghComment
		if err := json.Unmarshal(body, &comments); err != nil {
			return nil, fmt.Errorf("decode comments: %w", err)
		}
		for _, gc := range comments {
			createdAt, _ := time.Parse(time.RFC3339, gc.CreatedAt)
			if createdAt.Before(since) {
				continue
			}
			line := 0
			if gc.Line != nil {
				line = *gc.Line
			}
			out = append(out, &platforms.Comment{
				ID:        strconv.FormatInt(gc.ID, 10),
				Author:    gc.User.Login,
				Body:      gc.Body,
				FilePath:  gc.Path,
				Line:      line,
				CreatedAt: createdAt,
			})
		}
		nextURL = parseNextLink(resp.Header.Get("Link"))
	}
	return out, nil
}

func (c *Client) GetDiff(repo string, prID int) (string, error) {
	path := fmt.Sprintf("/repos/%s/pulls/%d", repo, prID)
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.diff")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return "", err
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c *Client) get(rawURL string) ([]byte, *http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp); err != nil {
		return nil, resp, err
	}

	body, err := io.ReadAll(resp.Body)
	return body, resp, err
}

func checkStatus(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusUnauthorized:
		return platforms.ErrUnauthorized
	case http.StatusNotFound:
		return platforms.ErrNotFound
	case http.StatusTooManyRequests:
		return platforms.ErrRateLimited
	default:
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
}

func parseNextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		segments := strings.Split(part, ";")
		if len(segments) != 2 {
			continue
		}
		if strings.TrimSpace(segments[1]) == `rel="next"` {
			link := strings.TrimSpace(segments[0])
			return strings.Trim(link, "<>")
		}
	}
	return ""
}
