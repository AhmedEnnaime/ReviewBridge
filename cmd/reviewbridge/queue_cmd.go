package main

import (
	"fmt"
	"io"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

func runQueue(out io.Writer, database *db.DB) error {
	queued, err := database.ListCommentsByState(db.CommentStateQueued)
	if err != nil {
		return fmt.Errorf("failed to read queued comments: %w", err)
	}
	parked, err := database.ListCommentsByState(db.CommentStateParked)
	if err != nil {
		return fmt.Errorf("failed to read parked comments: %w", err)
	}

	all := append(queued, parked...)
	if len(all) == 0 {
		fmt.Fprintln(out, "Queue is empty")
		return nil
	}

	byPR := make(map[string][]*db.Comment)
	var prOrder []string
	for _, c := range all {
		if _, exists := byPR[c.PRID]; !exists {
			prOrder = append(prOrder, c.PRID)
		}
		byPR[c.PRID] = append(byPR[c.PRID], c)
	}

	for _, prID := range prOrder {
		comments := byPR[prID]
		fmt.Fprintf(out, "PR #%s:\n", prNumStr(prID))
		for _, c := range comments {
			stateLabel := "[queued]"
			if c.State == db.CommentStateParked {
				stateLabel = "[parked]"
			}
			fmt.Fprintf(out, "  %s %-8s  %s:%d  @%s  %s\n",
				verdictIcon(c.TriageVerdict), stateLabel,
				c.FilePath, c.LineNumber, c.Author, truncate(c.Body, 60))
		}
		fmt.Fprintln(out)
	}
	return nil
}

func verdictIcon(verdict string) string {
	switch verdict {
	case db.VerdictFix:
		return "✅"
	case db.VerdictYourCall:
		return "⚠️ "
	case db.VerdictSkip:
		return "❌"
	default:
		return "  "
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
