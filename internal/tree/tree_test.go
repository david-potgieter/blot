package tree

import (
	"testing"

	"github.com/david-potgieter/heft/internal/scan"
)

func dir(name string, size int64, kids ...*scan.Node) *scan.Node {
	n := &scan.Node{Name: name, Path: "/" + name, IsDir: true, Size: size, Children: kids}
	for _, k := range kids {
		k.Parent = n
	}
	return n
}

func file(name string, size int64) *scan.Node {
	return &scan.Node{Name: name, Path: "/" + name, Size: size}
}

// root(1000)
//
//	big(600)
//	  nm(500)
//	    huge.bin(450)
//	  small.txt(10)
//	other(300)
//	  movie.mp4(290)
//	tiny(50)
func sample() *scan.Node {
	return dir("root", 1000,
		dir("big", 600, dir("nm", 500, file("huge.bin", 450)), file("small.txt", 10)),
		dir("other", 300, file("movie.mp4", 290)),
		dir("tiny", 50),
	)
}

func names(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Node.Name)
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func expandAll(tr *Tree) {
	for changed := true; changed; {
		changed = false
		for _, r := range tr.Rows() {
			if r.HasChildren && !r.Expanded {
				tr.Expand(r.Node)
				changed = true
			}
		}
	}
}

func TestRowsDefaultOnlyRootExpanded(t *testing.T) {
	tr := New([]*scan.Node{sample()}, Filter{Min: 280, Type: All})
	eq(t, names(tr.Rows()), []string{"root", "big", "other"})
}

func TestAllType(t *testing.T) {
	tr := New([]*scan.Node{sample()}, Filter{Min: 280, Type: All})
	expandAll(tr)
	eq(t, names(tr.Rows()), []string{"root", "big", "nm", "huge.bin", "other", "movie.mp4"})
}

func TestDirsType(t *testing.T) {
	tr := New([]*scan.Node{sample()}, Filter{Min: 280, Type: Dirs})
	expandAll(tr)
	eq(t, names(tr.Rows()), []string{"root", "big", "nm", "other"})
}

func TestFilesTypeShowsContextDirs(t *testing.T) {
	tr := New([]*scan.Node{sample()}, Filter{Min: 400, Type: Files})
	expandAll(tr)
	rows := tr.Rows()
	eq(t, names(rows), []string{"root", "big", "nm", "huge.bin"})
	for _, r := range rows {
		if want := r.Node.Name != "huge.bin"; r.Context != want {
			t.Errorf("%s context = %v, want %v", r.Node.Name, r.Context, want)
		}
	}
	if tr.MatchCount() != 1 {
		t.Errorf("matches = %d, want 1", tr.MatchCount())
	}
}

func TestRootBelowThresholdStillShown(t *testing.T) {
	tr := New([]*scan.Node{sample()}, Filter{Min: 5000, Type: All})
	rows := tr.Rows()
	eq(t, names(rows), []string{"root"})
	if !rows[0].Context || rows[0].HasChildren {
		t.Errorf("row = %+v", rows[0])
	}
}

func TestFilesTypeHidesBigDirWithoutBigFiles(t *testing.T) {
	root := dir("root", 1000,
		dir("many", 500, file("x", 10), file("y", 10)),
		dir("one", 450, file("big", 420)),
	)
	tr := New([]*scan.Node{root}, Filter{Min: 400, Type: Files})
	expandAll(tr)
	eq(t, names(tr.Rows()), []string{"root", "one", "big"})
}

func TestMultipleRootsAndMinZero(t *testing.T) {
	r1 := dir("r1", 10, file("a", 4), file("b", 6))
	r2 := dir("r2", 3, dir("c", 1))
	tr := New([]*scan.Node{r1, r2}, Filter{Min: 0, Type: All})
	eq(t, names(tr.Rows()), []string{"r1", "b", "a", "r2", "c"})
	if tr.MatchCount() != 5 {
		t.Errorf("matches = %d, want 5", tr.MatchCount())
	}
}

func TestSortTiesDeterministic(t *testing.T) {
	tr := New([]*scan.Node{dir("r", 10, file("b", 2), file("B", 2), file("a", 2))}, Filter{})
	eq(t, names(tr.Rows()), []string{"r", "a", "B", "b"})
}

func TestCollapseAndDepth(t *testing.T) {
	tr := New([]*scan.Node{sample()}, Filter{Min: 280, Type: All})
	expandAll(tr)
	rows := tr.Rows()
	if rows[3].Node.Name != "huge.bin" || rows[3].Depth != 3 {
		t.Errorf("row 3 = %s depth %d", rows[3].Node.Name, rows[3].Depth)
	}
	tr.Collapse(rows[1].Node) // big
	eq(t, names(tr.Rows()), []string{"root", "big", "other", "movie.mp4"})
}

func TestRebuildKeepsExpansionAndSort(t *testing.T) {
	tr := New([]*scan.Node{sample()}, Filter{Min: 280, Type: All})
	tr.Expand(tr.Rows()[1].Node) // big
	tr.SetSort(ByName)
	// A fresh copy of the same tree, as a new snapshot would give.
	tr2 := Rebuild([]*scan.Node{sample()}, tr.Filter(), tr.Sort(), tr.ExpandedPaths())
	eq(t, names(tr2.Rows()), []string{"root", "big", "nm", "other"})
	if tr2.Sort() != ByName {
		t.Error("sort not carried over")
	}
}

func TestSortSizeDesc(t *testing.T) {
	tr := New([]*scan.Node{dir("r", 10, dir("a", 1), dir("b", 5))}, Filter{})
	eq(t, names(tr.Rows()), []string{"r", "b", "a"})
	tr.SetSort(ByName)
	eq(t, names(tr.Rows()), []string{"r", "a", "b"})
}

func TestParseType(t *testing.T) {
	for s, want := range map[string]Type{"all": All, "dirs": Dirs, "files": Files, "DIRS": Dirs} {
		got, err := ParseType(s)
		if err != nil || got != want {
			t.Errorf("ParseType(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := ParseType("x"); err == nil {
		t.Error("expected error")
	}
}
