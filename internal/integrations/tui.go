package integrations

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type tuiModel struct {
	manager *Manager
	entries []IntegrationStatus
	cursor  int
	message string
	err     error
}

func NewTUIModel(manager *Manager) (tea.Model, error) {
	entries, err := manager.List()
	if err != nil {
		return nil, err
	}
	return tuiModel{
		manager: manager,
		entries: entries,
	}, nil
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.entries) == 0 {
				return m, nil
			}
			current := m.entries[m.cursor]
			if current.Installed {
				m.message = fmt.Sprintf("%s já está instalada.", current.Slug)
				m.err = nil
				return m, nil
			}
			entry, source, err := m.manager.Install(current.Slug)
			if err != nil {
				m.err = err
				m.message = ""
				return m, nil
			}
			m.entries, _ = m.manager.List()
			m.message = fmt.Sprintf("%s instalada via %s em integrations/%s.", entry.Slug, source, entry.RepoName)
			m.err = nil
		}
	}
	return m, nil
}

func (m tuiModel) View() string {
	var lines []string
	lines = append(lines, "Yggdrasil Integration Manager")
	lines = append(lines, "")
	lines = append(lines, "Setas ou j/k para navegar. Enter instala. q sai.")
	lines = append(lines, "")

	for i, entry := range m.entries {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}

		status := "available"
		if entry.Installed {
			status = "installed"
		}

		lines = append(lines, fmt.Sprintf("%s %s [%s/%s] - %s", cursor, entry.Slug, entry.Domain, entry.Section, status))
		lines = append(lines, fmt.Sprintf("  %s", entry.Description))
	}

	if m.message != "" {
		lines = append(lines, "")
		lines = append(lines, m.message)
	}

	if m.err != nil {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("erro: %v", m.err))
	}

	lines = append(lines, "")
	lines = append(lines, "Dica: exporte YGGDRASIL_INTEGRATIONS_DEV_DIR para preferir repositórios locais ao instalar.")
	return strings.Join(lines, "\n")
}
