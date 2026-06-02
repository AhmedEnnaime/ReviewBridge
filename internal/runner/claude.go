package runner

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/ahmedennaime/reviewbridge/internal/db"
)

const defaultTimeout = 10 * time.Minute

var (
	commitBracket = regexp.MustCompile(`\[[\w/.-]+ ([0-9a-f]{7,40})\]`)
	commitKeyword = regexp.MustCompile(`\bcommit ([0-9a-f]{7,40})\b`)
)

type RunResult struct {
	Output     string
	CommitHash string
	Duration   time.Duration
}

type Runner struct {
	timeout     time.Duration
	lookupFn    func(string) (string, error)
	listProcsFn func() ([]string, error)
}

func New() *Runner {
	return &Runner{
		timeout:     defaultTimeout,
		lookupFn:    exec.LookPath,
		listProcsFn: defaultListProcesses,
	}
}

func (r *Runner) WithTimeout(d time.Duration) *Runner {
	r.timeout = d
	return r
}

func (r *Runner) WithLookupFn(fn func(string) (string, error)) *Runner {
	r.lookupFn = fn
	return r
}

func (r *Runner) WithProcessLister(fn func() ([]string, error)) *Runner {
	r.listProcsFn = fn
	return r
}

func (r *Runner) IsClaudeInstalled() bool {
	_, err := r.lookupFn("claude")
	return err == nil
}

func (r *Runner) IsSessionActive(sessionID string) bool {
	procs, err := r.listProcsFn()
	if err != nil {
		return false
	}
	for _, line := range procs {
		if strings.Contains(line, "claude") &&
			strings.Contains(line, "--resume") &&
			strings.Contains(line, sessionID) {
			return true
		}
	}
	return false
}

func (r *Runner) Run(sessionID, prompt string) (*RunResult, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "--resume", sessionID, "-p", prompt)
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("claude timed out after %v", r.timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("claude failed: %w\noutput: %s", err, string(output))
	}

	outStr := string(output)
	return &RunResult{
		Output:     outStr,
		CommitHash: extractCommitHash(outStr),
		Duration:   duration,
	}, nil
}

func (r *Runner) ExtractCommit(output string) string {
	return extractCommitHash(output)
}

func (r *Runner) RunHeadless(prompt string) (*RunResult, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", prompt)
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("claude timed out after %v", r.timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("claude failed: %w\noutput: %s", err, string(output))
	}

	outStr := string(output)
	return &RunResult{
		Output:     outStr,
		CommitHash: extractCommitHash(outStr),
		Duration:   duration,
	}, nil
}

func BuildPrompt(comments []*db.Comment) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "You have %d approved review comments to address in this codebase.\n", len(comments))
	sb.WriteString("Please fix each one, then commit your changes.\n\n")
	for i, c := range comments {
		fmt.Fprintf(&sb, "%d. File: %s:%d\n   Author: @%s\n   Comment: %s\n\n",
			i+1, c.FilePath, c.LineNumber, c.Author, c.Body)
	}
	return sb.String()
}

func extractCommitHash(output string) string {
	var last string
	for _, re := range []*regexp.Regexp{commitBracket, commitKeyword} {
		for _, m := range re.FindAllStringSubmatch(output, -1) {
			if len(m) > 1 {
				last = m[1]
			}
		}
	}
	return last
}

func defaultListProcesses() ([]string, error) {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\n"), nil
}
