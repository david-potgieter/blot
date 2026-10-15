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
		Search:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "search")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k treeKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Expand, k.Collapse, k.Sort, k.Reveal, k.Rescan, k.Search, k.Quit}
}

// formHelp is the help line while the search pane has focus.
func formHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab/↑↓", "field")),
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "change")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search/cancel")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "results")),
		key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}
