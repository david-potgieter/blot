package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
