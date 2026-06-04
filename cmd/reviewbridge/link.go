package main

import (
	"fmt"
	"io"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

func runLink(out io.Writer, database *db.DB, sessionID, branch, prID string) error {
	if sessionID == "" {
		return fmt.Errorf("--session is required")
	}
	if branch == "" && prID == "" {
		return fmt.Errorf("either --branch or --pr is required")
	}

	s, err := database.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("failed to read session: %w", err)
	}
	if s == nil {
		return fmt.Errorf("session %q not found", sessionID)
	}

	if branch != "" {
		s.BranchName = branch
		if err := database.SaveSession(s); err != nil {
			return fmt.Errorf("failed to update session: %w", err)
		}
		database.TouchSession(sessionID) //nolint:errcheck
		prs, _ := database.ListOpenPullRequests()
		for _, pr := range prs {
			if pr.BranchName == branch {
				database.LinkPRToSession(pr.PRID, sessionID) //nolint:errcheck
			}
		}
		stale, _ := database.ListCommentsByStateAndBranch(db.CommentStateStaleSession, branch)
		for _, c := range stale {
			database.UpdateCommentState(c.CommentID, db.CommentStateQueued) //nolint:errcheck
		}
		if len(stale) > 0 {
			fmt.Fprintf(out, "Re-queued %d stale comment(s) for branch %s\n", len(stale), branch)
		}
		fmt.Fprintf(out, "Session %s linked to branch %s\n", sessionID, branch)
	}

	if prID != "" {
		if err := database.LinkPRToSession(prID, sessionID); err != nil {
			return fmt.Errorf("failed to link PR: %w", err)
		}
		fmt.Fprintf(out, "Session %s linked to PR %s\n", sessionID, prID)
	}

	return nil
}
