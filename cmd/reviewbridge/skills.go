package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const checkReviewsContent = `Check the ReviewBridge queue for pending review comments on the current branch.
Read the file at ~/.reviewbridge/queue/<current-branch>.json if it exists.
If there are pending comments, list them and ask which ones to fix.
If the file does not exist or is empty, report "no pending review comments on this branch".
`

func installSkill(out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not find home directory: %w", err)
	}

	destDir := filepath.Join(home, ".claude", "commands")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create commands directory: %w", err)
	}

	destPath := filepath.Join(destDir, "check-reviews.md")
	if err := os.WriteFile(destPath, []byte(checkReviewsContent), 0644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	fmt.Fprintf(out, "Installed /check-reviews skill to %s\n", destPath)
	return nil
}
