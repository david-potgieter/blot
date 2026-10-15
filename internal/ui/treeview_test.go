package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/david-potgieter/blot/internal/scan"
	"github.com/david-potgieter/blot/internal/tree"
)

func press(s string) tea.KeyPressMsg {
	switch s {
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// testRoots: /r(3MB) → a(2MB) → f.bin(1.9MB); /r → b(1MB).
func testRoots() []*scan.Node {
	root := &scan.Node{Name: "/r", Path: "/r", IsDir: true, Size: 3_000_000}
	a := &scan.Node{Name: "a", Path: "/r/a", IsDir: true, Size: 2_000_000, Parent: root}
	f := &scan.Node{Name: "f.bin", Path: "/r/a/f.bin", Size: 1_900_000, Parent: a}
	b := &scan.Node{Name: "b", Path: "/r/b", IsDir: true, Size: 1_000_000, Parent: root}
	a.Children = []*scan.Node{f}
	root.Children = []*scan.Node{b, a}
	return []*scan.Node{root}
}

func treeFor(roots []*scan.Node, minSize int64, w, h int) treeModel {
	m := newTreeModel(newStyles(true))
	m.setSize(w, h)
	m.setTree(tree.New(roots, tree.Filter{Min: minSize}))
	return m
}

func newTestTree() treeModel { return treeFor(testRoots(), 500_000, 100, 30) }

func send(m treeModel, keys ...string) treeModel {
	for _, k := range keys {
		m, _ = m.Update(press(k))
	}
	return m
}

func TestTreeNavigation(t *testing.T) {
	m := newTestTree()
	if got := m.current().Name; got != "/r" {
		t.Fatalf("start = %s", got)
	}
	m = send(m, "down")
	if got := m.current().Name; got != "a" {
		t.Fatalf("after down = %s (want size-sorted a first)", got)
	}
	m = send(m, "right", "down")
	if got := m.current().Name; got != "f.bin" {
		t.Fatalf("after expand+down = %s", got)
	}
	m = send(m, "left") // leaf → jump to parent
	if got := m.current().Name; got != "a" {
		t.Fatalf("after left on leaf = %s", got)
	}
	m = send(m, "left") // collapse a
	if len(m.rows) != 3 {
		t.Fatalf("rows after collapse = %d", len(m.rows))
	}
	m = send(m, "G")
	if got := m.current().Name; got != "b" {
		t.Fatalf("after G = %s", got)
	}
	m = send(m, "g", "up")
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want clamped 0", m.cursor)
	}
}

func TestTreeSortToggle(t *testing.T) {
	m := send(newTestTree(), "s", "down")
	if got := m.current().Name; got != "a" {
		t.Fatalf("name sort first child = %s", got)
	}
	if m.tree.Sort() != tree.ByName {
		t.Fatal("sort not toggled")
	}
}

func TestTreeSortKeepsCursorOnNode(t *testing.T) {
	// Size order is z, a; name order is a, z — so z changes row on sort.
	root := &scan.Node{Name: "/r", Path: "/r", IsDir: true, Size: 3_000_000}
	z := &scan.Node{Name: "z", Path: "/r/z", IsDir: true, Size: 2_000_000, Parent: root}
	a := &scan.Node{Name: "a", Path: "/r/a", IsDir: true, Size: 1_000_000, Parent: root}
	root.Children = []*scan.Node{a, z}
	m := treeFor([]*scan.Node{root}, 0, 100, 30)

	m = send(m, "down")
	if got := m.current().Name; got != "z" || m.cursor != 1 {
		t.Fatalf("before sort = %s at %d", got, m.cursor)
	}
	m = send(m, "s")
	if got := m.current().Name; got != "z" || m.cursor != 2 {
		t.Fatalf("after sort = %s at %d, want z at 2", got, m.cursor)
	}
}

func TestSetTreeKeepsCursorByPath(t *testing.T) {
	m := send(newTestTree(), "down", "down") // on b
	if m.current().Path != "/r/b" {
		t.Fatalf("start = %s", m.current().Path)
	}
	// A fresh snapshot where b is now bigger than a, so it moves to row 1.
	roots := testRoots()
	roots[0].Children[0].Size = 2_500_000
	m.setTree(tree.Rebuild(roots, tree.Filter{Min: 500_000}, tree.BySize, m.tree.ExpandedPaths()))
	if m.current().Path != "/r/b" || m.cursor != 1 {
		t.Errorf("cursor on %s at %d, want /r/b at 1", m.current().Path, m.cursor)
	}
}

func TestEmptyTreeIgnoresKeys(t *testing.T) {
	m := newTreeModel(newStyles(true))
	m.setSize(40, 5)
	m = send(m, "down", "right", "s", "o")
	if m.current() != nil || lipgloss.Height(m.View()) != 5 {
		t.Errorf("empty tree misbehaved")
	}
}

// bigTree has a root with n child dirs of descending size.
func bigTree(n, w, h int) treeModel {
	root := &scan.Node{Name: "/r", Path: "/r", IsDir: true, Size: int64(n * 1000)}
	for i := range n {
		c := &scan.Node{Name: fmt.Sprintf("d%02d", i), Path: fmt.Sprintf("/r/d%02d", i), IsDir: true, Size: int64((n - i) * 10), Parent: root}
		root.Children = append(root.Children, c)
	}
	return treeFor([]*scan.Node{root}, 0, w, h)
}

func TestTreeScrollMovesCursorBeforeWindow(t *testing.T) {
	m := send(bigTree(30, 80, 15), "G")
	if m.cursor != 30 {
		t.Fatalf("cursor = %d", m.cursor)
	}
	offset := m.offset
	if offset == 0 {
		t.Fatal("expected list to scroll at bottom")
	}
	m = send(m, "up")
	if m.offset != offset || m.cursor != 29 {
		t.Errorf("up from bottom: offset %d→%d, cursor %d; want window fixed", offset, m.offset, m.cursor)
	}
	m = send(m, "g")
	if m.offset != 0 {
		t.Errorf("offset after g = %d", m.offset)
	}
}

func TestTreeViewFitsBox(t *testing.T) {
	for _, keys := range [][]string{nil, {"G"}} {
		m := send(bigTree(30, 40, 15), keys...)
		out := m.View()
		if got := lipgloss.Height(out); got != 15 {
			t.Errorf("keys %v: height = %d, want 15", keys, got)
		}
		if got := lipgloss.Width(out); got > 40 {
			t.Errorf("keys %v: width = %d, want <= 40", keys, got)
		}
	}
}

func TestTreeViewRows(t *testing.T) {
	out := newTestTree().View()
	for _, want := range []string{"/r", "a/", "b/", "2.0 MB", "▸"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}
