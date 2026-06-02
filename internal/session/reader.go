package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)


var ErrEmptyFile = errors.New("session file is empty")

type Meta struct {
	SessionID string
	RepoPath  string
	CreatedAt time.Time
}

type jsonlEntry struct {
	CWD       string       `json:"cwd"`
	Timestamp string       `json:"timestamp"`
	Message   *jsonlMsg    `json:"message"`
}

type jsonlMsg struct {
	Content json.RawMessage `json:"content"`
}

type jsonlContent struct {
	Type  string         `json:"type"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func ReadMeta(path string) (*Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta := &Meta{
		SessionID: sessionIDFromPath(path),
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	hasLines := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		hasLines = true

		var entry jsonlEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if meta.CreatedAt.IsZero() && entry.Timestamp != "" {
			meta.CreatedAt, _ = time.Parse(time.RFC3339, entry.Timestamp)
			if meta.CreatedAt.IsZero() {
				meta.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000Z", entry.Timestamp)
			}
		}

		if meta.RepoPath == "" && entry.CWD != "" {
			meta.RepoPath = entry.CWD
		}

		if meta.RepoPath == "" && entry.Message != nil {
			var contents []jsonlContent
			if err := json.Unmarshal(entry.Message.Content, &contents); err == nil {
				for _, c := range contents {
					if c.Type == "tool_use" && c.Input != nil {
						if fp, ok := c.Input["file_path"].(string); ok && fp != "" {
							meta.RepoPath = filepath.Dir(fp)
						}
					}
				}
			}
		}

		if !meta.CreatedAt.IsZero() && meta.RepoPath != "" {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if !hasLines {
		return nil, ErrEmptyFile
	}

	return meta, nil
}

func sessionIDFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, ".jsonl")
}
