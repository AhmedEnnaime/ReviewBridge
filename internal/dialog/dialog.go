package dialog

import (
	tea "github.com/charmbracelet/bubbletea"
)

func Show(items []DialogItem) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}

	m := NewModel(items)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}

	result := final.(Model)
	if result.IsDismissed() {
		return nil, nil
	}
	return result.ApprovedIDs(), nil
}
