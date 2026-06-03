package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

func runStatus(out io.Writer, database *db.DB) error {
	sessions, err := database.ListActiveSessions()
	if err != nil {
		return fmt.Errorf("failed to read sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Fprintln(out, "No sessions tracked")
		fmt.Fprintln(out, "Start a Claude Code session and ReviewBridge will pick it up automatically.")
		return nil
	}

	prs, err := database.ListOpenPullRequests()
	if err != nil {
		return fmt.Errorf("failed to read pull requests: %w", err)
	}

	prsBySession := make(map[string][]*db.PullRequest)
	for _, pr := range prs {
		if pr.SessionID != nil {
			prsBySession[*pr.SessionID] = append(prsBySession[*pr.SessionID], pr)
		}
	}

	fmt.Fprintln(out, "Active sessions:")
	fmt.Fprintln(out)
	for _, s := range sessions {
		n := min(8, len(s.SessionID))
		shortID := s.SessionID[:n]
		fmt.Fprintf(out, "  %-10s  %-30s  %s\n", shortID, shortenPath(s.RepoPath), s.BranchName)
		for _, pr := range prsBySession[s.SessionID] {
			fmt.Fprintf(out, "               → PR #%s (%s) [%s]\n",
				prNumStr(pr.PRID), pr.Platform, pr.Status)
		}
	}
	return nil
}

func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func prNumStr(prID string) string {
	parts := strings.Split(prID, ":")
	if len(parts) < 3 {
		return prID
	}
	return parts[len(parts)-1]
}

