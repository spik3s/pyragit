package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// promptKind identifies what a prompt result should do.
type promptKind int

const (
	promptNone promptKind = iota
	promptSetBase
	promptCheckout
	promptNewBranch
	promptNewWorktree
	promptConfirm
)

// prompt is a modal text input with an optional filterable choice list.
type prompt struct {
	active   bool
	kind     promptKind
	title    string
	input    textinput.Model
	choices  []string
	filtered []string
	cursor   int
	confirm  bool // y/N style; input is ignored
	context  map[string]string
	err      string
}

func newPrompt() prompt {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.SetVirtualCursor(true)
	return prompt{input: ti}
}

func (p *prompt) open(kind promptKind, title string, choices []string, initial string) tea.Cmd {
	p.active = true
	p.kind = kind
	p.title = title
	p.choices = choices
	p.confirm = false
	p.err = ""
	p.context = map[string]string{}
	p.input.SetValue(initial)
	p.input.Placeholder = ""
	p.refilter()
	return p.input.Focus()
}

func (p *prompt) openConfirm(kind promptKind, title string, ctx map[string]string) {
	p.active = true
	p.kind = kind
	p.title = title
	p.choices = nil
	p.filtered = nil
	p.confirm = true
	p.err = ""
	p.context = ctx
	p.input.SetValue("")
	p.input.Blur()
}

func (p *prompt) close() {
	p.active = false
	p.kind = promptNone
	p.input.Blur()
}

func (p *prompt) refilter() {
	q := strings.ToLower(p.input.Value())
	p.filtered = p.filtered[:0]
	for _, c := range p.choices {
		if q == "" || strings.Contains(strings.ToLower(c), q) {
			p.filtered = append(p.filtered, c)
		}
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = max(len(p.filtered)-1, 0)
	}
}

// value returns the selected choice when the list has matches, else the typed text.
func (p *prompt) value() string {
	if len(p.filtered) > 0 && p.cursor < len(p.filtered) && (p.input.Value() == "" || p.kind == promptSetBase || p.kind == promptCheckout) {
		return p.filtered[p.cursor]
	}
	return strings.TrimSpace(p.input.Value())
}

// promptResultMsg is emitted into Update when the user submits a prompt.
type promptResultMsg struct {
	kind    promptKind
	value   string
	yes     bool
	context map[string]string
}

// update handles keys while the prompt is active. It returns a result message
// when submitted, and whether the prompt consumed the key.
func (p *prompt) update(msg tea.KeyPressMsg) (tea.Cmd, *promptResultMsg) {
	k := msg.String()
	if p.confirm {
		switch k {
		case "y", "Y":
			r := &promptResultMsg{kind: p.kind, yes: true, context: p.context}
			p.close()
			return nil, r
		case "n", "N", keyEsc, keyEnter, keyQuit:
			p.close()
			return nil, nil
		}
		return nil, nil
	}
	switch k {
	case keyEsc:
		p.close()
		return nil, nil
	case keyEnter:
		v := p.value()
		if v == "" {
			p.err = "value required"
			return nil, nil
		}
		r := &promptResultMsg{kind: p.kind, value: v, context: p.context}
		p.close()
		return nil, r
	case keyDown, "ctrl+n", keyTab:
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
		return nil, nil
	case keyUp, "ctrl+p", keyShiftTab:
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, nil
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.refilter()
	return cmd, nil
}

func (p *prompt) view(t Theme, width, height int) string {
	w := clamp(width-10, 30, 70)
	var b strings.Builder
	b.WriteString(t.TitleFocused.Render(p.title) + "\n")
	if p.confirm {
		b.WriteString("\n" + t.Key.Render("y") + t.Dim.Render(" confirm   ") + t.Key.Render("n") + t.Dim.Render(" cancel"))
	} else {
		p.input.SetWidth(w - 4)
		b.WriteString(p.input.View() + "\n")
		maxRows := clamp(height-10, 3, 12)
		start := 0
		if p.cursor >= maxRows {
			start = p.cursor - maxRows + 1
		}
		for i := start; i < len(p.filtered) && i < start+maxRows; i++ {
			line := padRight(ansi.Truncate(p.filtered[i], w-4, "…"), w-4)
			if i == p.cursor {
				line = t.Selected.Render(line)
			}
			b.WriteString("\n" + line)
		}
		if len(p.choices) > 0 && len(p.filtered) == 0 {
			b.WriteString("\n" + t.Dim.Render("no matches; enter uses typed value"))
		}
		if p.err != "" {
			b.WriteString("\n" + t.BadgeWarn.Render(p.err))
		}
		b.WriteString("\n\n" + t.Dim.Render("enter confirm · esc cancel"))
	}
	box := t.BorderFocus.Padding(0, 1).Width(w).Render(b.String())
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
