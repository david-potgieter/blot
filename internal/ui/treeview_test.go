package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/david-potgieter/heft/internal/scan"
	"github.com/david-potgieter/heft/internal/tree"
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

func testResult() *scan.Result {
	root := &scan.Node{Name: "/r", Path: "/r", IsDir: true, Size: 3_000_000}
	a := &scan.Node{Name: "a", Path: "/r/a", IsDir: true, Size: 2_000_000, Parent: root}
	f := &scan.Node{Name: "f.bin", Path: "/r/a/f.bin", Size: 1_900_000, Parent: a}
	b := &scan.Node{Name: "b", Path: "/r/b", IsDir: true, Size: 1_000_000, Parent: root}
	a.Children = []*scan.Node{f}
	root.Children = []*scan.Node{b, a}
	return &scan.Result{Roots: []*scan.Node{root}, Files: 10, Dirs: 3, Bytes: 3_000_000, ErrorCount: 2}
}

func newTestTree() treeModel {
	p := Params{Roots: []string{"/r"}, Min: 500_000, Type: tree.All}
	return newTreeModel(testResult(), p, newStyles(true), 100, 30)
}

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
	res := &scan.Result{Roots: []*scan.Node{root}}
	m := newTreeModel(res, Params{Roots: []string{"/r"}, Min: 0}, newStyles(true), 100, 30)

	m = send(m, "down")
	if got := m.current().Name; got != "z" || m.cursor != 1 {
		t.Fatalf("before sort = %s at %d", got, m.cursor)
	}
	m = send(m, "s")
	if got := m.current().Name; got != "z" || m.cursor != 2 {
		t.Fatalf("after sort = %s at %d, want z at 2", got, m.cursor)
	}
}

// bigTree has a root with n child dirs of descending size.
func bigTree(n, w, h int) treeModel {
	root := &scan.Node{Name: "/r", Path: "/r", IsDir: true, Size: int64(n * 1000)}
	for i := range n {
		c := &scan.Node{Name: fmt.Sprintf("d%02d", i), IsDir: true, Size: int64((n - i) * 10), Parent: root}
		root.Children = append(root.Children, c)
	}
	res := &scan.Result{Roots: []*scan.Node{root}, ErrorCount: 3}
	return newTreeModel(res, Params{Roots: []string{"/r"}}, newStyles(true), w, h)
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

func TestTreeViewFitsWindow(t *testing.T) {
	for _, keys := range [][]string{nil, {"G"}, {"?"}} {
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

func TestTreeView(t *testing.T) {
	out := newTestTree().View()
	for _, want := range []string{"heft", "/r", "a/", "b/", "2.0 MB", "2 errors", "Full Disk Access"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
}

func TestTreeQuit(t *testing.T) {
	_, cmd := newTestTree().Update(press("q"))
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q should return tea.Quit")
	}
}
