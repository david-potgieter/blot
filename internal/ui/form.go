package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/david-potgieter/heft/internal/size"
	"github.com/david-potgieter/heft/internal/tree"
)

type field int

const (
	fieldRoots field = iota
	fieldMin
	fieldType
	fieldButton
	numFields
)

var typeLabels = map[tree.Type]string{tree.All: "files & folders", tree.Dirs: "folders", tree.Files: "files"}

// submitMsg is sent when the Search/Cancel button is pressed.
type submitMsg struct{}

// formModel is the search form in the left pane.
type formModel struct {
	roots, min textinput.Model
	typ        tree.Type
	focus      field
	focused    bool // pane has keyboard focus
	scanning   bool // button shows Cancel
	styles     styles
	cache      *rootsCache // shared across copies; parseRoots stats the disk
}

type rootsCache struct {
	val   string
	roots []string
	err   error
	ok    bool
}

// parsedRoots parses the Folders field, re-checking the disk only when it changes.
func (f formModel) parsedRoots() ([]string, error) {
	v := f.roots.Value()
	if c := f.cache; c.ok && c.val == v {
		return c.roots, c.err
	}
	roots, err := parseRoots(v)
	*f.cache = rootsCache{val: v, roots: roots, err: err, ok: true}
	return roots, err
}

func newFormModel(v formValues, st styles) formModel {
	input := func(val, placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = placeholder
		ti.SetValue(val)
		return ti
	}
	return formModel{
		roots:  input(v.roots, "~/Work, ~/Downloads"),
		min:    input(v.min, "500MB"),
		typ:    v.typ,
		styles: st,
		cache:  &rootsCache{},
	}
}

func (f formModel) values() formValues {
	return formValues{roots: f.roots.Value(), min: f.min.Value(), typ: f.typ}
}

// setWidth fits the form into w cells. Inputs are indented 2 and textinput
// renders one cell wider than its width (for the cursor).
func (f *formModel) setWidth(w int) {
	f.roots.SetWidth(max(1, w-3))
	f.min.SetWidth(max(1, w-3))
}

// setFocused gives or takes keyboard focus for the whole pane.
func (f *formModel) setFocused(on bool) tea.Cmd {
	f.focused = on
	return f.syncFocus()
}

// syncFocus puts the text cursor in the active input, if any.
func (f *formModel) syncFocus() tea.Cmd {
	f.roots.Blur()
	f.min.Blur()
	if !f.focused {
		return nil
	}
	switch f.focus {
	case fieldRoots:
		return f.roots.Focus()
	case fieldMin:
		return f.min.Focus()
	}
	return nil
}

func (f formModel) Update(msg tea.Msg) (formModel, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var c1, c2 tea.Cmd
		f.roots, c1 = f.roots.Update(msg)
		f.min, c2 = f.min.Update(msg)
		return f, tea.Batch(c1, c2)
	}
	switch k.String() {
	case "tab", "down":
		f.focus = (f.focus + 1) % numFields
		return f, f.syncFocus()
	case "shift+tab", "up":
		f.focus = (f.focus + numFields - 1) % numFields
		return f, f.syncFocus()
	case "enter":
		return f, func() tea.Msg { return submitMsg{} }
	}
	var cmd tea.Cmd
	switch f.focus {
	case fieldRoots:
		f.roots, cmd = f.roots.Update(msg)
	case fieldMin:
		f.min, cmd = f.min.Update(msg)
	case fieldType:
		switch k.String() {
		case "left":
			f.typ = (f.typ + 2) % 3
		case "right", "space":
			f.typ = (f.typ + 1) % 3
		}
	}
	return f, cmd
}

func (f formModel) View() string {
	s := f.styles
	label := func(fl field, text string) string {
		if f.focused && f.focus == fl {
			return s.title.Render("› " + text)
		}
		return s.subtle.Render("  " + text)
	}
	var b strings.Builder

	b.WriteString(label(fieldRoots, "Folders") + "\n  " + f.roots.View() + "\n")
	if _, err := f.parsedRoots(); err != nil {
		b.WriteString("  " + s.err.Render(err.Error()) + "\n")
	}

	b.WriteString("\n" + label(fieldMin, "Min size") + "\n  " + f.min.View() + "\n")
	if _, err := size.Parse(f.min.Value()); err != nil {
		b.WriteString("  " + s.err.Render("try 500MB or 1GiB") + "\n")
	}

	b.WriteString("\n" + label(fieldType, "Show") + "\n  ‹ " + typeLabels[f.typ] + " ›\n\n")

	btn := "[ Search ]"
	if f.scanning {
		btn = "[ Cancel ]"
	}
	if f.focused && f.focus == fieldButton {
		btn = s.selected.Render(btn)
	} else {
		btn = s.title.Render(btn)
	}
	b.WriteString("  " + btn)
	return b.String()
}
