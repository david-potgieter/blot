package ui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// saveDialog prompts for a label to save the form under, prefilled with the
// Name pattern, or else the first folder. A label already taken asks first.
func (a *App) saveDialog() (*dialog, tea.Cmd) {
	label := strings.TrimSpace(a.form.name.Value())
	if label == "" {
		label = tildify(strings.TrimSpace(splitRoots(a.form.roots.Value())[0]), a.tree.home)
	}
	in := textinput.New()
	in.Prompt = ""
	in.SetStyles(textinput.DefaultStyles(a.isDark))
	in.SetValue(label)
	cmd := in.Focus()
	return &dialog{
		title: "Save query",
		input: &in,
		onSubmit: func(label string) tea.Cmd {
			if slices.Contains(a.form.saved, label) {
				a.dialog = a.overwriteDialog(label)
				return nil
			}
			a.saveQuery(label)
			return nil
		},
	}, cmd
}

// overwriteDialog confirms replacing the query saved under label.
func (a *App) overwriteDialog(label string) *dialog {
	return &dialog{
		title:   "Overwrite '" + label + "'?",
		body:    "A query is already saved under this name.",
		options: []string{"Yes", "No"},
		onPick: func(choice int) tea.Cmd {
			if choice == 0 {
				a.saveQuery(label)
			}
			return nil
		},
	}
}

// saveQuery saves the form under label; a failure shows where the list does.
func (a *App) saveQuery(label string) {
	if err := a.form.saveQuery(label); err != nil {
		a.form.savedErr = err
	}
}
