package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type Mode int

const (
	ModeFinder Mode = iota
	ModeGrep
)

type Model struct {
	mode   Mode
	query  string
	items  []string
	cursor int
	width  int
	height int
}

func NewModel() Model {
	return Model{
		mode: ModeFinder,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m Model) View() string {
	return "telescope-tui"
}
