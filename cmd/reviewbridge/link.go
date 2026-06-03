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
		prs, _ := database.ListOpenPullRequests()
		for _, pr := range prs {
			if pr.BranchName == branch {
				database.LinkPRToSession(pr.PRID, sessionID) //nolint:errcheck
			}
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
