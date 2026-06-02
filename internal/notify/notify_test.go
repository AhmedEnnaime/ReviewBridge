package notify_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ahmedennaime/reviewbridge/internal/notify"
)

func results(verdicts ...string) []notify.CommentResult {
	r := make([]notify.CommentResult, len(verdicts))
	for i, v := range verdicts {
		r[i] = notify.CommentResult{CommentID: fmt.Sprintf("c%d", i+1), Verdict: v}
	}
	return r
}

func TestNotifyFormatsSingleFix(t *testing.T) {
	body := notify.FormatBody(results("fix"))
	if !strings.Contains(body, "1 fix") {
		t.Errorf("body = %q, want to contain '1 fix'", body)
	}
}

func TestNotifyFormatsMultipleVerdicts(t *testing.T) {
	body := notify.FormatBody(results("fix", "fix", "your-call", "skip"))
	if !strings.Contains(body, "2 fix") {
		t.Errorf("body = %q, missing '2 fix'", body)
	}
	if !strings.Contains(body, "1 your-call") {
		t.Errorf("body = %q, missing '1 your-call'", body)
	}
	if !strings.Contains(body, "1 skip") {
		t.Errorf("body = %q, missing '1 skip'", body)
	}
}

func TestNotifyFormatsAllSkip(t *testing.T) {
	body := notify.FormatBody(results("skip", "skip", "skip"))
	if !strings.Contains(body, "nothing to fix") {
		t.Errorf("body = %q, want 'nothing to fix'", body)
	}
}

func TestNotifyFallsBackToTerminal(t *testing.T) {
	var buf bytes.Buffer
	n := notify.New().
		WithNotifyFn(func(_, _ string, _ any) error {
			return errors.New("desktop notification unavailable")
		}).
		WithOutput(&buf)

	n.Notify("ReviewBridge", "3 comments triaged")

	if buf.Len() == 0 {
		t.Error("expected terminal fallback output, got nothing")
	}
	if !strings.Contains(buf.String(), "ReviewBridge") {
		t.Errorf("fallback output = %q, want title in output", buf.String())
	}
}

func TestNotifyDoesNotPanicOnEmptyComments(t *testing.T) {
	n := notify.New().
		WithNotifyFn(func(_, _ string, _ any) error { return nil })

	n.NotifyComments(notify.PR{Number: 1, Branch: "main"}, nil)
	n.NotifyComments(notify.PR{Number: 1, Branch: "main"}, []notify.CommentResult{})
}
