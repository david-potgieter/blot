package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"

	"github.com/david-potgieter/heft/internal/scan"
	"github.com/david-potgieter/heft/internal/size"
	"github.com/david-potgieter/heft/internal/tree"
)

const barWidth = 10

// treeModel is the navigable result list. It renders rows only, into a box
// sized by setSize; the app draws the pane around it.
type treeModel struct {
	tree          *tree.Tree
	rows          []tree.Row
	cursor        int
	offset        int
	width, height int
	keys          treeKeys
	styles        styles
	home          string // roots under home display as ~/…
}

func newTreeModel(st styles) treeModel {
	home, _ := os.UserHomeDir()
	return treeModel{keys: newTreeKeys(), styles: st, home: home}
}

// setTree swaps in a rebuilt tree, keeping the cursor on the same path.
func (m *treeModel) setTree(t *tree.Tree) {
	var cur string
	if n := m.current(); n != nil {
		cur = n.Path
	}
	m.tree = t
	m.rows = nil
	if t != nil {
		m.rows = t.Rows()
	}
	// Follow the node; if it was filtered out, fall back to its nearest visible ancestor.
	index := make(map[string]int, len(m.rows))
	for i, r := range m.rows {
		index[r.Node.Path] = i
	}
	for p := cur; p != ""; {
		if i, ok := index[p]; ok {
			m.cursor = i
			break
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	m.refresh()
}

func (m *treeModel) setSize(w, h int) {
	m.width, m.height = w, h
	m.refresh()
}

// refresh rebuilds rows, clamps the cursor and scrolls just enough to keep it visible.
func (m *treeModel) refresh() {
	if m.tree != nil {
		m.rows = m.tree.Rows()
	}
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

func (m treeModel) listHeight() int {
	if m.height <= 0 {
		return max(1, len(m.rows))
	}
	return m.height
}

// current is the node under the cursor, or nil when there are no rows.
func (m treeModel) current() *scan.Node {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].Node
}

func (m treeModel) Update(msg tea.Msg) (treeModel, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok || len(m.rows) == 0 {
		return m, nil
	}
	switch {
	case key.Matches(k, m.keys.Up):
		m.cursor--
	case key.Matches(k, m.keys.Down):
		m.cursor++
	case key.Matches(k, m.keys.Top):
		m.cursor = 0
	case key.Matches(k, m.keys.Bottom):
		m.cursor = len(m.rows) - 1
	case key.Matches(k, m.keys.Expand):
		m.tree.Expand(m.current())
	case key.Matches(k, m.keys.Collapse):
		m.collapse()
	case key.Matches(k, m.keys.Sort):
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
	case key.Matches(k, m.keys.Reveal):
		return m, reveal(m.current().Path)
	}
	m.refresh()
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

// View renders exactly listHeight lines, each at most width cells.
func (m treeModel) View() string {
	h := m.listHeight()
	end := min(len(m.rows), m.offset+h)
	lines := make([]string, 0, h)
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], i == m.cursor))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// fit truncates a line to the box width so it never wraps.
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
	if r.Depth == 0 {
		name = tildify(name, m.home)
	} else if n.IsDir {
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
