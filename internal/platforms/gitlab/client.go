package gitlab

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

const defaultInstanceURL = "https://gitlab.com"

type Client struct {
	token   string
	apiBase string
	http    *http.Client
}

func New(token, instanceURL string) *Client {
	if instanceURL == "" {
		instanceURL = defaultInstanceURL
	}
	return &Client{
		token:   token,
		apiBase: strings.TrimRight(instanceURL, "/") + "/api/v4",
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

type glMR struct {
	IID          int    `json:"iid"`
	Title        string `json:"title"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	WebURL       string `json:"web_url"`
}

type glNote struct {
	ID     int    `json:"id"`
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
	Body      string   `json:"body"`
	CreatedAt string   `json:"created_at"`
	Position  *glPos   `json:"position"`
}

type glPos struct {
	NewPath string `json:"new_path"`
	NewLine int    `json:"new_line"`
}

type glChange struct {
	Diff string `json:"diff"`
	NewPath string `json:"new_path"`
}

func (c *Client) ListOpenPullRequests(repo string) ([]*platforms.PullRequest, error) {
	encoded := url.PathEscape(repo)
	path := fmt.Sprintf("/projects/%s/merge_requests?state=opened&per_page=100", encoded)
	var out []*platforms.PullRequest
	page := 1
	for {
		body, resp, err := c.get(fmt.Sprintf("%s%s&page=%d", c.apiBase, path, page))
		if err != nil {
			return nil, err
		}
		var mrs []glMR
		if err := json.Unmarshal(body, &mrs); err != nil {
			return nil, fmt.Errorf("decode MRs: %w", err)
		}
		for _, mr := range mrs {
			out = append(out, &platforms.PullRequest{
				Number:       mr.IID,
				Title:        mr.Title,
				SourceBranch: mr.SourceBranch,
				State:        mr.State,
				HTMLURL:      mr.WebURL,
			})
		}
		if resp.Header.Get("X-Next-Page") == "" {
			break
		}
		next, err := strconv.Atoi(resp.Header.Get("X-Next-Page"))
		if err != nil {
			break
		}
		page = next
	}
	return out, nil
}

func (c *Client) GetPullRequest(repo string, prID int) (*platforms.PullRequest, error) {
	encoded := url.PathEscape(repo)
	path := fmt.Sprintf("/projects/%s/merge_requests/%d", encoded, prID)
	body, _, err := c.get(c.apiBase + path)
	if err != nil {
		return nil, err
	}
	var mr glMR
	if err := json.Unmarshal(body, &mr); err != nil {
		return nil, fmt.Errorf("decode MR: %w", err)
	}
	return &platforms.PullRequest{
		Number:       mr.IID,
		Title:        mr.Title,
		SourceBranch: mr.SourceBranch,
		State:        mr.State,
		HTMLURL:      mr.WebURL,
	}, nil
}

func (c *Client) ListCommentsSince(repo string, prID int, since time.Time) ([]*platforms.Comment, error) {
	encoded := url.PathEscape(repo)
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/notes?per_page=100", encoded, prID)
	var out []*platforms.Comment
	page := 1
	for {
		body, resp, err := c.get(fmt.Sprintf("%s%s&page=%d", c.apiBase, path, page))
		if err != nil {
			return nil, err
		}
		var notes []glNote
		if err := json.Unmarshal(body, &notes); err != nil {
			return nil, fmt.Errorf("decode notes: %w", err)
		}
		for _, n := range notes {
			createdAt, _ := time.Parse(time.RFC3339, n.CreatedAt)
			if createdAt.Before(since) {
				continue
			}
			comment := &platforms.Comment{
				ID:        strconv.Itoa(n.ID),
				Author:    n.Author.Username,
				Body:      n.Body,
				CreatedAt: createdAt,
			}
			if n.Position != nil {
				comment.FilePath = n.Position.NewPath
				comment.Line = n.Position.NewLine
			}
			out = append(out, comment)
		}
		if resp.Header.Get("X-Next-Page") == "" {
			break
		}
		next, err := strconv.Atoi(resp.Header.Get("X-Next-Page"))
		if err != nil {
			break
		}
		page = next
	}
	return out, nil
}

func (c *Client) GetDiff(repo string, prID int) (string, error) {
	encoded := url.PathEscape(repo)
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/changes", encoded, prID)
	body, _, err := c.get(c.apiBase + path)
	if err != nil {
		return "", err
	}
	var result struct {
		Changes []glChange `json:"changes"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode changes: %w", err)
	}
	var sb strings.Builder
	for _, ch := range result.Changes {
		fmt.Fprintf(&sb, "--- %s\n+++ %s\n%s", ch.NewPath, ch.NewPath, ch.Diff)
	}
	return sb.String(), nil
}

func (c *Client) get(rawURL string) ([]byte, *http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")

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
