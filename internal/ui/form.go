package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
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
	listed     *dirList    // shared across copies; suggestions read the disk
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
	roots := input(v.roots, "~/Work, ~/Downloads")
	roots.ShowSuggestions = true
	// ↑/↓ move between fields, so only ctrl+n/p cycle matches. Tab is accepted
	// by the form, not the input: the input keeps the case as typed ("~/doc"
	// → "~/documents/"), and the app only passes tab on when there's a match.
	roots.KeyMap.NextSuggestion = key.NewBinding(key.WithKeys("ctrl+n"))
	roots.KeyMap.PrevSuggestion = key.NewBinding(key.WithKeys("ctrl+p"))
	roots.KeyMap.AcceptSuggestion = key.NewBinding(key.WithDisabled())
	f := formModel{
		roots:  roots,
		min:    input(v.min, "500MB"),
		typ:    v.typ,
		styles: st,
		cache:  &rootsCache{},
		listed: &dirList{},
	}
	f.suggest()
	return f
}

// maxSuggestions caps the matches handed to the input, which re-matches them
// on every keystroke; folders like node_modules can hold thousands.
const maxSuggestions = 50

// dirList caches the subfolders of the folder last completed in, so typing
// within one folder reads it once. It is shared across copies.
type dirList struct {
	dir          string
	names, lower []string // lower is names lowercased, for matching
}

// subdirs lists the folders (and symlinks to folders) in dir, sorted, with
// their lowercased names.
func (l *dirList) subdirs(dir string) (names, lower []string) {
	if l.dir == dir && l.names != nil {
		return l.names, l.lower
	}
	entries, _ := os.ReadDir(dir)
	names, lower = []string{}, []string{}
	for _, e := range entries {
		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 {
			st, err := os.Stat(filepath.Join(dir, e.Name()))
			isDir = err == nil && st.IsDir()
		}
		if isDir {
			names = append(names, e.Name())
			lower = append(lower, strings.ToLower(e.Name()))
		}
	}
	l.dir, l.names, l.lower = dir, names, lower
	return names, lower
}

// suggest offers folders that complete the last path in Folders. Suggestions
// are whole field values, since the input matches them against everything typed.
func (f *formModel) suggest() {
	v := f.roots.Value()
	last := strings.TrimLeft(v[strings.LastIndex(v, ",")+1:], " ")
	if last == "" {
		f.roots.SetSuggestions(nil)
		return
	}
	head := v[:len(v)-len(last)]
	slash := strings.LastIndex(last, "/") + 1
	typedDir, base := last[:slash], last[slash:]
	if base == "" {
		// Nothing typed of the next name yet: offering its first subfolder
		// would make tab complete forever instead of leaving the field.
		f.roots.SetSuggestions(nil)
		return
	}

	dir := typedDir
	switch {
	case dir == "":
		dir = "."
	case strings.HasPrefix(dir, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			f.roots.SetSuggestions(nil)
			return
		}
		dir = filepath.Join(home, dir[2:])
	}

	var out []string
	names, lower := f.listed.subdirs(dir)
	lowerBase := strings.ToLower(base)
	for i, name := range names {
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue // hidden folders only once a dot is typed
		}
		if name != base && strings.HasPrefix(lower[i], lowerBase) {
			out = append(out, head+typedDir+name+"/")
			if len(out) == maxSuggestions {
				break
			}
		}
	}
	f.roots.SetSuggestions(out)
}

// canComplete reports whether tab would complete Folders rather than move on.
func (f formModel) canComplete() bool {
	if !f.focused || f.focus != fieldRoots {
		return false
	}
	s := f.roots.CurrentSuggestion()
	return s != "" && len(s) > len(f.roots.Value())
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
		before := f.roots.Value()
		f.roots, c1 = f.roots.Update(msg)
		f.min, c2 = f.min.Update(msg)
		if f.roots.Value() != before { // pasted
			f.suggest()
		}
		return f, tea.Batch(c1, c2)
	}
	switch k.String() {
	case "down":
		f.focus = (f.focus + 1) % numFields
		return f, f.syncFocus()
	case "up":
		f.focus = (f.focus + numFields - 1) % numFields
		return f, f.syncFocus()
	case "enter":
		return f, func() tea.Msg { return submitMsg{} }
	case "tab":
		if f.canComplete() {
			f.roots.SetValue(f.roots.CurrentSuggestion())
			f.roots.CursorEnd()
			f.suggest()
		}
		return f, nil
	}
	var cmd tea.Cmd
	switch f.focus {
	case fieldRoots:
		before := f.roots.Value()
		f.roots, cmd = f.roots.Update(msg)
		if f.roots.Value() != before {
			f.suggest()
		}
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
