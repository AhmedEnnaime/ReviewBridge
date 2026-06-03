package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const checkReviewsContent = `Check the ReviewBridge queue for pending review comments on the current branch.

Steps:
1. Run ` + "`git branch --show-current`" + ` to get the current branch name.
2. Derive the queue file path: replace every ` + "`/`" + ` in the branch name with ` + "`-`" + `, then read ` + "`~/.reviewbridge/queue/<safe-branch>.json`" + `. For example, branch ` + "`feature/issue-a`" + ` → file ` + "`~/.reviewbridge/queue/feature-issue-a.json`" + `.
3. If the file does not exist or contains an empty ` + "`comments`" + ` array, respond: "No pending review comments on this branch."
4. If comments are present, display a numbered list. For each comment show:
   - Verdict icon: ✅ for ` + "`fix`" + `, ⚠️ for ` + "`your-call`" + `, ❌ for ` + "`skip`" + `
   - File path and line number
   - Author
   - Comment body
5. Ask: "Which comments would you like to fix? Enter numbers, 'all', or 'none'."
6. For each approved comment, apply the fix directly in this session and commit the changes.
`

func installSkill(out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not find home directory: %w", err)
	}
	return installSkillTo(filepath.Join(home, ".claude", "commands"), out)
}

func installSkillTo(destDir string, out io.Writer) error {
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
