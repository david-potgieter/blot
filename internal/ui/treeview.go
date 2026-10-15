package ui

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"

	"github.com/david-potgieter/heft/internal/scan"
	"github.com/david-potgieter/heft/internal/size"
	"github.com/david-potgieter/heft/internal/tree"
)

const barWidth = 10

type (
	rescanMsg    struct{}
	newSearchMsg struct{}
)

type treeModel struct {
	tree          *tree.Tree
	res           *scan.Result
	params        Params
	rows          []tree.Row
	cursor        int
	offset        int
	width, height int
	keys          treeKeys
	help          help.Model
	styles        styles
}

func newTreeModel(res *scan.Result, p Params, st styles, w, h int) treeModel {
	m := treeModel{
		tree:   tree.New(res.Roots, tree.Filter{Min: p.Min, Type: p.Type}),
		res:    res,
		params: p,
		width:  w,
		height: h,
		keys:   newTreeKeys(),
		help:   help.New(),
		styles: st,
	}
	m.help.SetWidth(w)
	m.refresh()
	return m
}

// refresh rebuilds rows, clamps the cursor and scrolls just enough to keep it visible.
func (m *treeModel) refresh() {
	m.rows = m.tree.Rows()
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(0, min(m.offset, len(m.rows)-h))
}

// listHeight is the number of rows that fit between header and footer.
func (m treeModel) listHeight() int {
	if m.height <= 0 {
		return max(1, len(m.rows))
	}
	return max(1, m.height-3-lipgloss.Height(m.help.View(m.keys)))
}

func (m treeModel) current() *scan.Node { return m.rows[m.cursor].Node }

func (m treeModel) Update(msg tea.Msg) (treeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.refresh()
	case tea.KeyPressMsg:
		if len(m.rows) == 0 {
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}
			return m, nil
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Up):
			m.cursor--
		case key.Matches(msg, m.keys.Down):
			m.cursor++
		case key.Matches(msg, m.keys.Top):
			m.cursor = 0
		case key.Matches(msg, m.keys.Bottom):
			m.cursor = len(m.rows) - 1
		case key.Matches(msg, m.keys.Expand):
			m.tree.Expand(m.current())
		case key.Matches(msg, m.keys.Collapse):
			m.collapse()
		case key.Matches(msg, m.keys.Sort):
			cur := m.current()
			if m.tree.Sort() == tree.BySize {
				m.tree.SetSort(tree.ByName)
			} else {
				m.tree.SetSort(tree.BySize)
			}
			// Keep the cursor on the same node after rows are reordered.
			m.rows = m.tree.Rows()
			for i, r := range m.rows {
				if r.Node == cur {
					m.cursor = i
					break
				}
			}
		case key.Matches(msg, m.keys.Reveal):
			return m, reveal(m.current().Path)
		case key.Matches(msg, m.keys.Rescan):
			return m, func() tea.Msg { return rescanMsg{} }
		case key.Matches(msg, m.keys.New):
			return m, func() tea.Msg { return newSearchMsg{} }
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		}
		m.refresh()
	}
	return m, nil
}

// collapse closes the current row, or jumps to its parent row.
func (m *treeModel) collapse() {
	row := m.rows[m.cursor]
	if row.Expanded {
		m.tree.Collapse(row.Node)
		return
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.rows[i].Node == row.Node.Parent {
			m.cursor = i
			return
		}
	}
}

func reveal(path string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command("open", "-R", path).Run()
		return nil
	}
}

func (m treeModel) View() string {
	s := m.styles
	header := s.title.Render("heft") + "  " + s.subtle.Render(m.params.summary()+" · sort: "+m.tree.Sort().String())

	footer := fmt.Sprintf("scanned %s in %s files · %d matches",
		size.Format(m.res.Bytes), humanize.Comma(m.res.Files), m.tree.MatchCount())
	if m.res.ErrorCount > 0 {
		footer += " · " + s.err.Render(fmt.Sprintf("%d errors — grant your terminal Full Disk Access for complete results", m.res.ErrorCount))
	}
	helpView := m.help.View(m.keys)

	listHeight := m.listHeight()
	end := min(len(m.rows), m.offset+listHeight)

	var b strings.Builder
	b.WriteString(m.fit(header) + "\n\n")
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(m.rows[i], i == m.cursor) + "\n")
	}
	for i := end - m.offset; i < listHeight; i++ {
		b.WriteString("\n")
	}
	b.WriteString(m.fit(footer) + "\n" + helpView)
	return b.String()
}

// fit truncates a line to the window width so it never wraps.
func (m treeModel) fit(line string) string {
	if m.width <= 0 {
		return line
	}
	return lipgloss.NewStyle().MaxWidth(m.width).Render(line)
}

func (m treeModel) renderRow(r tree.Row, selected bool) string {
	s := m.styles
	n := r.Node

	marker := "  "
	if r.HasChildren {
		marker = "▸ "
		if r.Expanded {
			marker = "▾ "
		}
	}
	name := n.Name
	if n.IsDir && r.Depth > 0 {
		name += "/"
	}
	sz := fmt.Sprintf("%9s", size.Format(n.Size))
	filled := barFill(n)
	full, empty := strings.Repeat("█", filled), strings.Repeat("░", barWidth-filled)
	indent := strings.Repeat("  ", r.Depth)
	mod := humanize.Time(n.ModTime)

	// The selected row is styled as one plain string: inner style resets would
	// otherwise cancel the reverse highlight.
	if selected {
		return m.fit(s.selected.Render(fmt.Sprintf("%s %s%s %s%s%s  %s", sz, full, empty, indent, marker, name, mod)))
	}
	nameStyle := lipgloss.NewStyle()
	if r.Context {
		nameStyle = s.context
	}
	return m.fit(fmt.Sprintf("%s %s %s%s%s  %s",
		s.size.Render(sz),
		s.bar.Render(full)+s.barEmpty.Render(empty),
		indent,
		s.marker.Render(marker),
		nameStyle.Render(name),
		s.subtle.Render(mod),
	))
}

// barFill is how many of barWidth cells n's share of its parent fills (roots are full).
func barFill(n *scan.Node) int {
	filled := barWidth
	if p := n.Parent; p != nil && p.Size > 0 {
		filled = int(float64(n.Size)/float64(p.Size)*barWidth + 0.5)
	}
	return max(0, min(barWidth, filled))
}
