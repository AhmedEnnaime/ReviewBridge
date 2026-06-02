package dialog_test

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ahmedennaime/reviewbridge/internal/dialog"
)

func items(verdicts ...string) []dialog.DialogItem {
	result := make([]dialog.DialogItem, len(verdicts))
	for i, v := range verdicts {
		result[i] = dialog.DialogItem{
			CommentID: fmt.Sprintf("c%d", i+1),
			Author:    "alice",
			Body:      fmt.Sprintf("comment %d", i+1),
			FilePath:  "main.go",
			Line:      i + 1,
			Verdict:   v,
		}
	}
	return result
}

func sendKey(m dialog.Model, key string) dialog.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(dialog.Model)
}

func sendSpecialKey(m dialog.Model, t tea.KeyType) dialog.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: t})
	return updated.(dialog.Model)
}

func TestDialogInitialState(t *testing.T) {
	m := dialog.NewModel(items("fix", "fix", "your-call", "skip"))
	fix, yourCall, skip := m.Counts()
	if fix != 2 {
		t.Errorf("fix count = %d, want 2", fix)
	}
	if yourCall != 1 {
		t.Errorf("your-call count = %d, want 1", yourCall)
	}
	if skip != 1 {
		t.Errorf("skip count = %d, want 1", skip)
	}
	if m.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0", m.Cursor())
	}
}

func TestDialogToggleOverride(t *testing.T) {
	m := dialog.NewModel(items("fix", "your-call", "skip"))

	m = sendSpecialKey(m, tea.KeyDown)
	m = sendSpecialKey(m, tea.KeyDown)

	if m.Cursor() != 2 {
		t.Fatalf("cursor = %d, want 2", m.Cursor())
	}
	if m.ItemVerdict(2) != "skip" {
		t.Fatalf("pre-toggle verdict = %q, want skip", m.ItemVerdict(2))
	}

	m = sendKey(m, " ")

	if m.ItemVerdict(2) != "fix" {
		t.Errorf("post-toggle verdict = %q, want fix", m.ItemVerdict(2))
	}
}

func TestDialogToggleOverrideBack(t *testing.T) {
	m := dialog.NewModel(items("fix", "your-call", "skip"))

	m = sendSpecialKey(m, tea.KeyDown)
	m = sendSpecialKey(m, tea.KeyDown)

	m = sendKey(m, " ")
	if m.ItemVerdict(2) != "fix" {
		t.Fatalf("after first toggle = %q, want fix", m.ItemVerdict(2))
	}

	m = sendKey(m, " ")
	if m.ItemVerdict(2) != "skip" {
		t.Errorf("after second toggle = %q, want skip (original)", m.ItemVerdict(2))
	}
}

func TestDialogApproveReturnsFixedComments(t *testing.T) {
	m := dialog.NewModel(items("fix", "fix", "your-call", "skip"))
	m = sendSpecialKey(m, tea.KeyEnter)

	ids := m.ApprovedIDs()
	if len(ids) != 2 {
		t.Errorf("got %d approved IDs, want 2", len(ids))
	}
}

func TestDialogApproveExcludesSkipped(t *testing.T) {
	m := dialog.NewModel(items("skip", "skip", "skip"))
	m = sendSpecialKey(m, tea.KeyEnter)

	ids := m.ApprovedIDs()
	if len(ids) != 0 {
		t.Errorf("got %d approved IDs, want 0 (all skipped)", len(ids))
	}
}

func TestDialogDismissReturnsEmpty(t *testing.T) {
	m := dialog.NewModel(items("fix", "fix"))
	m = sendKey(m, "q")

	if !m.IsDismissed() {
		t.Error("expected model to be dismissed after Q")
	}
	if len(m.ApprovedIDs()) != 0 {
		t.Error("expected no approved IDs after dismiss")
	}
}

func TestDialogEmptyInput(t *testing.T) {
	m := dialog.NewModel(nil)
	if !m.IsEmpty() {
		t.Error("expected IsEmpty() = true for empty input")
	}
}
