package dialog

import (
	"github.com/charmbracelet/bubbletea"
	"github.com/ahmedennaime/reviewbridge/internal/db"
)

type DialogItem struct {
	CommentID string
	Author    string
	Body      string
	FilePath  string
	Line      int
	Verdict   string
}

type item struct {
	DialogItem
	origVerdict string
}

type state int

const (
	stateBrowsing state = iota
	stateApproved
	stateDismissed
)

type Model struct {
	items  []item
	cursor int
	state  state
}

func NewModel(items []DialogItem) Model {
	converted := make([]item, len(items))
	for i, di := range items {
		converted[i] = item{DialogItem: di, origVerdict: di.Verdict}
	}
	return Model{items: converted}
}

func FromComments(comments []*db.Comment) Model {
	items := make([]DialogItem, len(comments))
	for i, c := range comments {
		items[i] = DialogItem{
			CommentID: c.CommentID,
			Author:    c.Author,
			Body:      c.Body,
			FilePath:  c.FilePath,
			Line:      c.LineNumber,
			Verdict:   c.TriageVerdict,
		}
	}
	return NewModel(items)
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.state != stateBrowsing {
		return m, tea.Quit
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case " ":
		if len(m.items) > 0 {
			cur := &m.items[m.cursor]
			if cur.Verdict == verdictFix {
				if cur.origVerdict == verdictFix {
					cur.Verdict = verdictSkip
				} else {
					cur.Verdict = cur.origVerdict
				}
			} else {
				cur.Verdict = verdictFix
			}
		}
	case "enter":
		m.state = stateApproved
		return m, tea.Quit
	case "q", "Q", "esc":
		m.state = stateDismissed
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) IsEmpty() bool {
	return len(m.items) == 0
}

func (m Model) IsDismissed() bool {
	return m.state == stateDismissed
}

func (m Model) ApprovedIDs() []string {
	if m.state == stateDismissed {
		return nil
	}
	var ids []string
	for _, it := range m.items {
		if it.Verdict == verdictFix {
			ids = append(ids, it.CommentID)
		}
	}
	return ids
}

func (m Model) Counts() (fix, yourCall, skip int) {
	for _, it := range m.items {
		switch it.Verdict {
		case verdictFix:
			fix++
		case verdictYourCall:
			yourCall++
		default:
			skip++
		}
	}
	return
}

func (m Model) Cursor() int {
	return m.cursor
}

func (m Model) ItemVerdict(i int) string {
	if i < 0 || i >= len(m.items) {
		return ""
	}
	return m.items[i].Verdict
}

const (
	verdictFix      = "fix"
	verdictYourCall = "your-call"
	verdictSkip     = "skip"
)
