package ui

import "charm.land/bubbles/v2/key"

type treeKeys struct {
	Up, Down, Expand, Collapse, Top, Bottom, Sort, Reveal, Rescan, New, Help, Quit key.Binding
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
		Reveal:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "reveal in Finder")),
		Rescan:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		New:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new search")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k treeKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Expand, k.Collapse, k.Sort, k.Reveal, k.Help, k.Quit}
}

func (k treeKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Expand, k.Collapse, k.Sort},
		{k.Reveal, k.Rescan, k.New},
		{k.Help, k.Quit},
	}
}
