// Package tree filters, sorts and flattens a scan tree into display rows.
package tree

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/david-potgieter/heft/internal/scan"
)

// Type selects which kinds of nodes match.
type Type int

const (
	All Type = iota
	Dirs
	Files
)

var typeNames = [...]string{"all", "dirs", "files"}

func (t Type) String() string {
	if t < 0 || int(t) >= len(typeNames) {
		return fmt.Sprintf("Type(%d)", int(t))
	}
	return typeNames[t]
}

// ParseType parses "all", "dirs" or "files".
func ParseType(s string) (Type, error) {
	for i, n := range typeNames {
		if strings.EqualFold(s, n) {
			return Type(i), nil
		}
	}
	return All, fmt.Errorf("invalid type %q (want all, dirs or files)", s)
}

// Filter decides which nodes match.
type Filter struct {
	Min  int64
	Type Type
}

// Matches reports whether n satisfies the size and type constraints.
func (f Filter) Matches(n *scan.Node) bool {
	if n.Size < f.Min {
		return false
	}
	switch f.Type {
	case Dirs:
		return n.IsDir
	case Files:
		return !n.IsDir
	}
	return true
}

// SortBy orders siblings.
type SortBy int

const (
	BySize SortBy = iota
	ByName
)

func (s SortBy) String() string {
	if s == ByName {
		return "name"
	}
	return "size"
}

// Row is one visible line. Context rows are shown only to reach matches.
type Row struct {
	Node        *scan.Node
	Depth       int
	Expanded    bool
	HasChildren bool
	Context     bool
}

// Tree holds visibility, sort and expansion state over scan roots.
type Tree struct {
	roots    []*scan.Node
	filter   Filter
	sort     SortBy
	kids     map[*scan.Node][]*scan.Node // visible children, sorted
	expanded map[string]bool             // by path, so state survives rebuilds from new snapshots
	matches  int
}

// New computes visibility once. Roots are always shown and start expanded.
func New(roots []*scan.Node, f Filter) *Tree {
	return Rebuild(roots, f, BySize, nil)
}

// Rebuild is New with a sort order and expanded paths carried over from a
// previous tree (see ExpandedPaths). A nil expanded map expands the roots.
func Rebuild(roots []*scan.Node, f Filter, s SortBy, expanded map[string]bool) *Tree {
	t := &Tree{roots: roots, filter: f, sort: s, kids: map[*scan.Node][]*scan.Node{}, expanded: map[string]bool{}}
	for p, e := range expanded {
		t.expanded[p] = e
	}
	for _, r := range roots {
		t.visit(r)
		if expanded == nil {
			t.expanded[r.Path] = true
		}
	}
	t.sortAll()
	return t
}

// ExpandedPaths returns a copy of the expanded paths, for Rebuild.
func (t *Tree) ExpandedPaths() map[string]bool {
	out := make(map[string]bool, len(t.expanded))
	for p, e := range t.expanded {
		out[p] = e
	}
	return out
}

// visit caches visible children of n and reports whether n is visible.
// Sizes are monotonic up the tree, so nothing under a node below Min can match.
func (t *Tree) visit(n *scan.Node) bool {
	if n.Size < t.filter.Min {
		return false
	}
	var kids []*scan.Node
	for _, c := range n.Children {
		if t.visit(c) {
			kids = append(kids, c)
		}
	}
	if len(kids) > 0 {
		t.kids[n] = kids
	}
	if t.filter.Matches(n) {
		t.matches++
		return true
	}
	return n.IsDir && len(kids) > 0
}

// Filter returns the filter the tree was built with.
func (t *Tree) Filter() Filter { return t.filter }

// Sort returns the current sibling order.
func (t *Tree) Sort() SortBy { return t.sort }

// MatchCount is the number of nodes matching the filter.
func (t *Tree) MatchCount() int { return t.matches }

// Roots returns the roots in the order given; they are never re-sorted.
func (t *Tree) Roots() []*scan.Node { return t.roots }

// SetSort reorders all visible children (not roots).
func (t *Tree) SetSort(s SortBy) {
	t.sort = s
	t.sortAll()
}

func (t *Tree) sortAll() {
	for _, k := range t.kids {
		slices.SortStableFunc(k, t.compare)
	}
}

func (t *Tree) compare(a, b *scan.Node) int {
	if t.sort == BySize {
		if c := cmp.Compare(b.Size, a.Size); c != 0 {
			return c
		}
	}
	// Exact name breaks case-only ties, since scan order varies between runs.
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(a.Name, b.Name),
	)
}

// Expand opens n if it has visible children.
func (t *Tree) Expand(n *scan.Node) {
	if len(t.kids[n]) > 0 {
		t.expanded[n.Path] = true
	}
}

// Collapse closes n.
func (t *Tree) Collapse(n *scan.Node) { delete(t.expanded, n.Path) }

// Rows flattens visible, expanded nodes depth-first.
func (t *Tree) Rows() []Row {
	var rows []Row
	var walk func(n *scan.Node, depth int)
	walk = func(n *scan.Node, depth int) {
		kids := t.kids[n]
		exp := t.expanded[n.Path] && len(kids) > 0
		rows = append(rows, Row{Node: n, Depth: depth, Expanded: exp, HasChildren: len(kids) > 0, Context: !t.filter.Matches(n)})
		if exp {
			for _, c := range kids {
				walk(c, depth+1)
			}
		}
	}
	for _, r := range t.roots {
		walk(r, 0)
	}
	return rows
}
