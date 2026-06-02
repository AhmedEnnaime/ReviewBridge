package triage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

const (
	maxDiffChars  = 32000
	systemPrompt  = `You are a code review triage assistant. Analyze each review comment on a pull request and decide what should happen to it.

Verdict options:
- "fix": clear bug, security issue, or obvious correctness problem — should be fixed
- "your-call": valid point but architectural, ambiguous, or needs a team decision
- "skip": style nitpick, personal preference, or not relevant to the current work

Return ONLY a JSON array. No explanation, no markdown fences. Each element must have:
- "comment_id": the exact ID string provided
- "verdict": one of "fix", "your-call", "skip"
- "reason": one short sentence explaining why`
)

func BuildPrompt(diff string, comments []*db.Comment, repoPath string) string {
	var sb strings.Builder

	sb.WriteString("## Pull Request Diff\n\n")
	sb.WriteString(truncateDiff(diff))
	sb.WriteString("\n\n")

	sb.WriteString("## Review Comments\n\n")
	for _, c := range comments {
		fmt.Fprintf(&sb, "---\nID: %s\nAuthor: @%s\nFile: %s:%d\nBody: %s\n\n",
			c.CommentID, c.Author, c.FilePath, c.LineNumber, c.Body)
	}

	claudeMD := loadCLAUDEMD(repoPath)
	sb.WriteString("## Project Guidelines (CLAUDE.md)\n\n")
	if claudeMD != "" {
		sb.WriteString(claudeMD)
	} else {
		sb.WriteString("(No CLAUDE.md found)")
	}

	sb.WriteString("\n\nReturn the JSON array of verdicts now.")
	return sb.String()
}

func SystemPrompt() string {
	return systemPrompt
}

func truncateDiff(diff string) string {
	if len(diff) <= maxDiffChars {
		return diff
	}
	files := extractChangedFiles(diff)
	header := ""
	if len(files) > 0 {
		header = "Changed files: " + strings.Join(files, ", ") + "\n\n"
	}
	truncated := diff[:maxDiffChars]
	remaining := len(diff) - maxDiffChars
	note := fmt.Sprintf("\n\n[Diff truncated. Showing first %d chars. %d more chars not shown.]",
		maxDiffChars, remaining)
	return header + truncated + note
}

func extractChangedFiles(diff string) []string {
	seen := map[string]bool{}
	var files []string
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			f := strings.TrimPrefix(line, "+++ b/")
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	return files
}

func loadCLAUDEMD(repoPath string) string {
	if repoPath == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoPath, "CLAUDE.md"))
	if err != nil {
		return ""
	}
	return string(data)
}
