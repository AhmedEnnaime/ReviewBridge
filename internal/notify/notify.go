package notify

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gen2brain/beeep"
)

type PR struct {
	Number int
	Branch string
}

type CommentResult struct {
	CommentID string
	Verdict   string
}

type Notifier struct {
	notifyFn func(title, body string, icon any) error
	out      io.Writer
}

func New() *Notifier {
	return &Notifier{
		notifyFn: beeep.Notify,
		out:      os.Stdout,
	}
}

func (n *Notifier) WithNotifyFn(fn func(title, body string, icon any) error) *Notifier {
	n.notifyFn = fn
	return n
}

func (n *Notifier) WithOutput(w io.Writer) *Notifier {
	n.out = w
	return n
}

func (n *Notifier) Notify(title, body string) {
	if err := n.notifyFn(title, body, ""); err != nil {
		fmt.Fprintf(n.out, "\n╔════════════════════════════════════╗\n")
		fmt.Fprintf(n.out, "  %s\n", title)
		fmt.Fprintf(n.out, "  %s\n", body)
		fmt.Fprintf(n.out, "╚════════════════════════════════════╝\n\n")
	}
}

func (n *Notifier) NotifyComments(pr PR, results []CommentResult) {
	if len(results) == 0 {
		return
	}
	title := fmt.Sprintf("ReviewBridge — PR #%d (%s)", pr.Number, pr.Branch)
	body := FormatBody(results)
	n.Notify(title, body)
}

func FormatBody(results []CommentResult) string {
	if len(results) == 0 {
		return ""
	}

	var fix, yourCall, skip int
	for _, r := range results {
		switch r.Verdict {
		case "fix":
			fix++
		case "your-call":
			yourCall++
		default:
			skip++
		}
	}

	if fix == 0 && yourCall == 0 {
		return fmt.Sprintf("%d comments triaged: nothing to fix", len(results))
	}

	var parts []string
	if fix > 0 {
		parts = append(parts, fmt.Sprintf("✅ %d fix", fix))
	}
	if yourCall > 0 {
		parts = append(parts, fmt.Sprintf("⚠️  %d your-call", yourCall))
	}
	if skip > 0 {
		parts = append(parts, fmt.Sprintf("❌ %d skip", skip))
	}

	return fmt.Sprintf("%d comments triaged: %s", len(results), strings.Join(parts, " · "))
}
