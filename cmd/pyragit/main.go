// Command pyragit is a terminal UI for managing many git projects and their worktrees.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

type helloModel struct {
	width, height int
}

func (m helloModel) Init() tea.Cmd { return nil }

func (m helloModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m helloModel) View() tea.View {
	v := tea.NewView(fmt.Sprintf("pyragit %dx%d — press q to quit", m.width, m.height))
	v.AltScreen = true
	return v
}

func main() {
	if _, err := tea.NewProgram(helloModel{}).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "pyragit:", err)
		os.Exit(1)
	}
}
