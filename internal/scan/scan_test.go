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

func TestScanHardlinkCountedOnce(t *testing.T) {
	root := fixture(t)
	f2 := filepath.Join(root, "a", "b", "f2")
	if err := os.Link(f2, filepath.Join(root, "f2link")); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(context.Background(), []string{root}, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// du() counts the hard link twice; scan must count it once.
	if got, want := res.Roots[0].Size, du(t, root)-du(t, f2); got != want {
		t.Errorf("size = %d, want %d", got, want)
	}
	if res.Files != 3 {
		t.Errorf("files = %d, want 3", res.Files)
	}
}

func TestScanSymlinkNotFollowed(t *testing.T) {
	root := fixture(t)
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	writeFile(t, filepath.Join(outside, "big"), 1_000_000)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(context.Background(), []string{root}, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Roots[0].Size, du(t, root); got != want {
		t.Errorf("size = %d, want %d (symlink target must not count)", got, want)
	}
}

func TestScanExclude(t *testing.T) {
	root := fixture(t)
	a := filepath.Join(root, "a")
	res, err := Scan(context.Background(), []string{root}, Options{Exclude: []string{a}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Roots[0].Children {
		if c.Name == "a" {
			t.Error("excluded dir present")
		}
	}
	if got, want := res.Roots[0].Size, du(t, root)-du(t, a); got != want {
		t.Errorf("size = %d, want %d", got, want)
	}
}

func TestScanPermissionErrorIsNotFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	root := fixture(t)
	locked := filepath.Join(root, "locked")
	writeFile(t, filepath.Join(locked, "x"), 1000)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	res, err := Scan(context.Background(), []string{root}, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCount < 1 || len(res.Errors) < 1 {
		t.Errorf("errors = %d, want >= 1", res.ErrorCount)
	}
}

func TestScanCancelled(t *testing.T) {
	root := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, []string{root}, Options{}, nil); err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestScanSendsProgress(t *testing.T) {
	root := fixture(t)
	ch := make(chan Progress, 1)
	if _, err := Scan(context.Background(), []string{root}, Options{}, ch); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-ch:
		if p.Files == 0 {
			t.Errorf("progress = %+v", p)
		}
	default:
		t.Error("no progress sent")
	}
}
