// Package ui is the Bubble Tea front end.
package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/spik3s/pyragit/internal/config"
)

// App is the root model.
type App struct {
	cfg     config.Config
	cfgPath string
	created bool
	width   int
	height  int
}

// New builds the root model.
func New(cfg config.Config, cfgPath string, created bool) App {
	return App{cfg: cfg, cfgPath: cfgPath, created: created}
}

func (a App) Init() tea.Cmd { return nil }

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		}
	}
	return a, nil
}

func (a App) View() tea.View {
	v := tea.NewView(fmt.Sprintf("pyragit %dx%d — press q to quit", a.width, a.height))
	v.AltScreen = true
	return v
}
