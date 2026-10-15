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

func (m *treeModel) refresh() {
	m.rows = m.tree.Rows()
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
}

func (m treeModel) current() *scan.Node { return m.rows[m.cursor].Node }

func (m treeModel) Update(msg tea.Msg) (treeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
	case tea.KeyPressMsg:
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
			for i, r := range m.tree.Rows() {
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

	listHeight := len(m.rows)
	if m.height > 0 {
		listHeight = max(1, m.height-3-lipgloss.Height(helpView))
	}
	offset := m.offset
	if m.cursor < offset {
		offset = m.cursor
	}
	if m.cursor >= offset+listHeight {
		offset = m.cursor - listHeight + 1
	}
	end := min(len(m.rows), offset+listHeight)

	var b strings.Builder
	b.WriteString(header + "\n\n")
	for i := offset; i < end; i++ {
		b.WriteString(m.renderRow(m.rows[i], i == m.cursor) + "\n")
	}
	for i := end - offset; i < listHeight; i++ {
		b.WriteString("\n")
	}
	b.WriteString(footer + "\n" + helpView)
	return b.String()
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
	nameStyle := lipgloss.NewStyle()
	if r.Context {
		nameStyle = s.context
	}

	line := fmt.Sprintf("%s %s %s%s%s  %s",
		s.size.Render(fmt.Sprintf("%9s", size.Format(n.Size))),
		m.bar(n),
		strings.Repeat("  ", r.Depth),
		s.marker.Render(marker),
		nameStyle.Render(name),
		s.subtle.Render(humanize.Time(n.ModTime)),
	)
	if selected {
		line = s.selected.Render(line)
	}
	if m.width > 0 {
		line = lipgloss.NewStyle().MaxWidth(m.width).Render(line)
	}
	return line
}

// bar shows n's share of its parent (roots are full).
func (m treeModel) bar(n *scan.Node) string {
	filled := barWidth
	if p := n.Parent; p != nil && p.Size > 0 {
		filled = int(float64(n.Size)/float64(p.Size)*barWidth + 0.5)
	}
	filled = max(0, min(barWidth, filled))
	return m.styles.bar.Render(strings.Repeat("█", filled)) + m.styles.barEmpty.Render(strings.Repeat("░", barWidth-filled))
}
