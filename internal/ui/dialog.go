package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const dialogWidth = 56 // including border and padding

// dialog is a modal choice between buttons. It takes all keys while open.
type dialog struct {
	title, body string
	options     []string
	choice      int
	onPick      func(choice int) tea.Cmd
}

// update handles a key; done reports that the dialog should close.
func (d *dialog) update(msg tea.KeyPressMsg) (cmd tea.Cmd, done bool) {
	switch msg.String() {
	case "left", "h", "shift+tab":
		d.choice = (d.choice + len(d.options) - 1) % len(d.options)
	case "right", "l", "tab":
		d.choice = (d.choice + 1) % len(d.options)
	case "esc", "q":
		return nil, true
	case "enter":
		return d.onPick(d.choice), true
	}
	return nil, false
}

// view renders the dialog at most w cells wide. Body lines are cut to fit.
func (d *dialog) view(st styles, w int) string {
	w = min(dialogWidth, w)
	lines := strings.Split(d.body, "\n")
	for i, l := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(max(1, w-4)).Render(l)
	}
	btns := make([]string, len(d.options))
	for i, o := range d.options {
		label := "[ " + o + " ]"
		if i == d.choice {
			btns[i] = st.selected.Render(label)
		} else {
			btns[i] = st.title.Render(label)
		}
	}
	body := st.title.Render(d.title) + "\n\n" +
		st.subtle.Render(strings.Join(lines, "\n")) + "\n\n" +
		strings.Join(btns, "  ")
	return st.focusedPane.Width(w).Render(body)
}
