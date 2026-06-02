package dialog

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleCursor  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleVerdict = map[string]lipgloss.Style{
		verdictFix:      lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		verdictYourCall: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		verdictSkip:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	}
	styleHeader = lipgloss.NewStyle().Bold(true)
	styleFooter = lipgloss.NewStyle().Faint(true)
)

var verdictIcon = map[string]string{
	verdictFix:      "✅",
	verdictYourCall: "⚠️ ",
	verdictSkip:     "❌",
}

func (m Model) View() string {
	if m.state != stateBrowsing {
		return ""
	}

	var sb strings.Builder

	if m.IsEmpty() {
		sb.WriteString(styleHeader.Render("ReviewBridge"))
		sb.WriteString("\n\nNo new comments to review.\n")
		sb.WriteString(styleFooter.Render("\n[Q] dismiss"))
		return sb.String()
	}

	fix, yourCall, skip := m.Counts()
	header := fmt.Sprintf("ReviewBridge — %d comments triaged: ✅ %d fix · ⚠️  %d your-call · ❌ %d skip",
		len(m.items), fix, yourCall, skip)
	sb.WriteString(styleHeader.Render(header))
	sb.WriteString("\n\n")

	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = styleCursor.Render("▶ ")
		}

		icon := verdictIcon[it.Verdict]
		if icon == "" {
			icon = "❓"
		}

		style, ok := styleVerdict[it.Verdict]
		if !ok {
			style = lipgloss.NewStyle()
		}

		body := it.Body
		if len(body) > 60 {
			body = body[:57] + "..."
		}

		line := fmt.Sprintf("%s%s  @%s  %s:%d  %s",
			cursor, icon, it.Author, it.FilePath, it.Line, body)
		sb.WriteString(style.Render(line))
		sb.WriteString("\n")
	}

	sb.WriteString(styleFooter.Render("\n[↑↓] navigate  [Space] toggle  [Enter] approve  [Q] dismiss"))
	return sb.String()
}
