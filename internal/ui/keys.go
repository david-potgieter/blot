package ui

import "charm.land/bubbles/v2/key"

// treeKeys are the results pane bindings. Rescan, Search and Quit are handled
// by the app but listed here for the help line.
type treeKeys struct {
	Up, Down, Expand, Collapse, Top, Bottom, Sort, Reveal, Rescan, Search, Quit key.Binding
}

func newTreeKeys() treeKeys {
	return treeKeys{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Expand:   key.NewBinding(key.WithKeys("right", "l", "enter"), key.WithHelp("→/l", "expand")),
		Collapse: key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		Sort:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
		Reveal:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "reveal")),
		Rescan:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Search:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "pane")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k treeKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Expand, k.Collapse, k.Sort, k.Reveal, k.Rescan, k.Search, k.Quit}
}

// dialogHelp is the help line while a dialog is open.
func dialogHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "choose")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

// formHelp is the help line while the search pane has focus. While a folder
// completion is showing, tab completes it instead of moving to the next pane.
func formHelp(completing bool) []key.Binding {
	tab := key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "pane"))
	if completing {
		tab = key.NewBinding(key.WithKeys("tab", "ctrl+n"), key.WithHelp("tab/^n", "complete/next"))
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "field")),
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "change")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search/cancel")),
		tab,
		key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}
