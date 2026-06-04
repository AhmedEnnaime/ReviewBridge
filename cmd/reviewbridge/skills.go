package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const checkReviewsContent = `Check the ReviewBridge queue for pending review comments on the current branch and fix them.

Steps:
1. Run ` + "`git branch --show-current`" + ` to get the current branch name.
2. Derive the queue file path: replace every ` + "`/`" + ` in the branch name with ` + "`-`" + `, then read ` + "`~/.reviewbridge/queue/<safe-branch>.json`" + `. For example, branch ` + "`feature/issue-a`" + ` → file ` + "`~/.reviewbridge/queue/feature-issue-a.json`" + `.
3. If the file does not exist or contains an empty ` + "`comments`" + ` array, respond: "No pending review comments on this branch."
4. If comments are present, display a numbered list. For each comment show:
   - Verdict icon: ✅ for ` + "`fix`" + ` (must fix), ⚠️ for ` + "`your-call`" + ` (optional, use judgment), ❌ for ` + "`skip`" + ` (no action needed)
   - File path and line number
   - Author and comment body
5. For ` + "`fix`" + ` comments: apply the fix directly in this session without asking — these were already triaged.
6. For ` + "`your-call`" + ` comments: briefly explain the trade-off and ask whether to apply it before proceeding.
7. Do NOT commit. Leave the changes unstaged so the user can review the diff, adjust if needed, and commit themselves.
8. Once all comments have been handled (applied or confirmed already applied), delete the queue file at the path from step 2.
9. Report what was fixed and what was skipped.
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
