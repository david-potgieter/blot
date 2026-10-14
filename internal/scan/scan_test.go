package scan

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// du sums on-disk usage (st_blocks*512) of path and everything under it, not following symlinks.
func du(t *testing.T, path string) int64 {
	t.Helper()
	var total int64
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Sys().(*syscall.Stat_t).Blocks * 512
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func writeFile(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture: root/a/f1 (10KB), root/a/b/f2 (200KB), root/f3 (5KB)
func fixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "a", "f1"), 10_000)
	writeFile(t, filepath.Join(root, "a", "b", "f2"), 200_000)
	writeFile(t, filepath.Join(root, "f3"), 5_000)
	return root
}

func child(t *testing.T, n *Node, name string) *Node {
	t.Helper()
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%s has no child %q", n.Path, name)
	return nil
}

func TestScanSizes(t *testing.T) {
	root := fixture(t)
	res, err := Scan(context.Background(), []string{root}, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Roots) != 1 {
		t.Fatalf("roots = %d", len(res.Roots))
	}
	r := res.Roots[0]
	if r.Name != root || !r.IsDir {
		t.Errorf("root node = %+v", r)
	}
	if got, want := r.Size, du(t, root); got != want {
		t.Errorf("root size = %d, want %d", got, want)
	}
	a := child(t, r, "a")
	if got, want := a.Size, du(t, filepath.Join(root, "a")); got != want {
		t.Errorf("a size = %d, want %d", got, want)
	}
	if a.Parent != r {
		t.Error("a.Parent != root")
	}
	child(t, r, "f3")
	if res.Files != 3 || res.Dirs != 3 {
		t.Errorf("files=%d dirs=%d, want 3, 3", res.Files, res.Dirs)
	}
}

func TestScanFileFloorPrunesSmallFiles(t *testing.T) {
	root := fixture(t)
	res, err := Scan(context.Background(), []string{root}, Options{FileFloor: 100_000}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := res.Roots[0]
	for _, c := range r.Children {
		if c.Name == "f3" {
			t.Error("f3 below floor should not be a child")
		}
	}
	b := child(t, child(t, r, "a"), "b")
	child(t, b, "f2")
	if got, want := r.Size, du(t, root); got != want {
		t.Errorf("pruned files must still count: size = %d, want %d", got, want)
	}
}

func TestScanFileRoot(t *testing.T) {
	root := fixture(t)
	f := filepath.Join(root, "f3")
	res, err := Scan(context.Background(), []string{f}, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := res.Roots[0]
	if n.IsDir || n.Size != du(t, f) || n.Name != f {
		t.Errorf("file root = %+v", n)
	}
}

func TestScanMissingRoot(t *testing.T) {
	_, err := Scan(context.Background(), []string{"/nope/not/here"}, Options{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
