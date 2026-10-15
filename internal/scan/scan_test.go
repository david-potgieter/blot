package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
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

func TestRemove(t *testing.T) {
	root := fixture(t)
	other := fixture(t)
	job := Start(context.Background(), []string{root, other}, Options{FileFloor: 100_000})
	before, err := job.Result()
	if err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "a")
	aSize := du(t, a)
	aFiles := du(t, filepath.Join(a, "f1")) + du(t, filepath.Join(a, "b", "f2"))

	job.Remove(a, filepath.Join(other, "f3"), root, "/nope", filepath.Join(root, "missing"))
	got := job.Snapshot(0)
	if got[0].Size != du(t, root)-aSize {
		t.Errorf("root size = %d, want %d", got[0].Size, du(t, root)-aSize)
	}
	for _, c := range got[0].Children {
		if c.Name == "a" {
			t.Error("a still in tree")
		}
	}
	// f3 is below the floor, so it has no node and stays counted.
	if got[1].Size != du(t, other) {
		t.Errorf("other size = %d, want unchanged %d", got[1].Size, du(t, other))
	}
	p := job.Progress()
	if p.Files != before.Files-2 || p.Dirs != before.Dirs-2 || p.Bytes != before.Bytes-aFiles {
		t.Errorf("files=%d dirs=%d bytes=%d, want %d %d %d",
			p.Files, p.Dirs, p.Bytes, before.Files-2, before.Dirs-2, before.Bytes-aFiles)
	}

	f2 := filepath.Join(other, "a", "b", "f2") // a file with a node
	job.Remove(f2)
	if got := job.Snapshot(0)[1]; got.Size != du(t, other)-du(t, f2) {
		t.Errorf("other size after file removal = %d", got.Size)
	}
}

func TestRestore(t *testing.T) {
	root := fixture(t)
	orig := Start(context.Background(), []string{root}, Options{FileFloor: 100_000})
	if _, err := orig.Result(); err != nil {
		t.Fatal(err)
	}
	job := Restore(orig.Snapshot(0), orig.Progress(), 100_000)
	select {
	case <-job.Done():
	default:
		t.Fatal("restored job not done")
	}
	if got, want := fmt.Sprint(tree(job.Snapshot(0)[0])), fmt.Sprint(tree(orig.Snapshot(0)[0])); got != want {
		t.Errorf("restored tree\n%s\nwant\n%s", got, want)
	}
	want := orig.Progress()
	want.Current = "" // nothing is being walked
	if job.Progress() != want {
		t.Errorf("progress = %+v, want %+v", job.Progress(), want)
	}
	a := filepath.Join(root, "a")
	job.Remove(a) // works on a restored job, and file counts came across
	if p := job.Progress(); p.Files != 1 || p.Dirs != 1 {
		t.Errorf("after remove files=%d dirs=%d, want 1, 1", p.Files, p.Dirs)
	}
}

// tree lists path=size/files for n and everything under it.
func tree(n *Node) []string {
	out := []string{fmt.Sprintf("%s=%d/%d/%d", n.Path, n.Size, n.Files, n.FileBytes)}
	for _, c := range n.Children {
		out = append(out, tree(c)...)
	}
	return out
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

func TestScanUnreadableRootFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	root := fixture(t)
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o755) })

	if _, err := Scan(context.Background(), []string{root}, Options{}, nil); err == nil {
		t.Error("expected an error for an unreadable root, not an empty result")
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

// cancelAfter reports Canceled once Err has been called more than limit times.
type cancelAfter struct {
	context.Context
	calls atomic.Int64
	limit int64
}

func (c *cancelAfter) Err() error {
	if c.calls.Add(1) > c.limit {
		return context.Canceled
	}
	return nil
}

func TestScanCancelledMidWalk(t *testing.T) {
	root := fixture(t)
	ctx := &cancelAfter{Context: context.Background(), limit: 3}
	if _, err := Scan(ctx, []string{root}, Options{}, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if ctx.calls.Load() <= 2 {
		t.Error("walk never checked ctx")
	}
}

func TestScanExcludeResolvesSymlinks(t *testing.T) {
	root := fixture(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	res, err := Scan(context.Background(), []string{root}, Options{Exclude: []string{filepath.Join(alias, "a")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Roots[0].Children {
		if c.Name == "a" {
			t.Error("exclude given via symlinked path was not applied")
		}
	}
}

// checkSums asserts every dir is at least as big as its children combined.
func checkSums(t *testing.T, n *Node) {
	t.Helper()
	var sum int64
	for _, c := range n.Children {
		sum += c.Size
		if c.Parent != n {
			t.Fatalf("%s: wrong parent", c.Path)
		}
		checkSums(t, c)
	}
	if sum > n.Size {
		t.Fatalf("%s: size %d < children %d", n.Path, n.Size, sum)
	}
}

func TestSnapshotsDuringScanAreConsistent(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	for d := range 40 {
		for f := range 25 {
			writeFile(t, filepath.Join(root, fmt.Sprintf("d%02d", d), "sub", fmt.Sprintf("f%02d", f)), 5_000)
		}
	}
	job := Start(context.Background(), []string{root}, Options{})
	snaps := 0
	for {
		select {
		case <-job.Done():
			if snaps == 0 {
				t.Log("scan finished before any mid-scan snapshot")
			}
			res, err := job.Result()
			if err != nil {
				t.Fatal(err)
			}
			checkSums(t, res.Roots[0])
			if got, want := res.Roots[0].Size, du(t, root); got != want {
				t.Errorf("final size = %d, want %d", got, want)
			}
			return
		default:
			for _, r := range job.Snapshot(0) {
				checkSums(t, r)
			}
			snaps++
		}
	}
}

func TestSnapshotPrunesBelowMin(t *testing.T) {
	root := fixture(t)
	job := Start(context.Background(), []string{root}, Options{})
	if _, err := job.Result(); err != nil {
		t.Fatal(err)
	}
	r := job.Snapshot(100_000)[0]
	if len(r.Children) != 1 || r.Children[0].Name != "a" {
		t.Fatalf("root children = %v", r.Children)
	}
	b := child(t, r.Children[0], "b")
	child(t, b, "f2")
	if len(b.Children) != 1 {
		t.Errorf("b children = %d, want only f2", len(b.Children))
	}
	if got := len(job.Snapshot(1 << 40)); got != 1 {
		t.Errorf("roots always kept: got %d", got)
	}
}

func TestCancelledJobKeepsPartialResult(t *testing.T) {
	root := fixture(t)
	ctx := &cancelAfter{Context: context.Background(), limit: 4}
	res, err := Start(ctx, []string{root}, Options{}).Result()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if res == nil || len(res.Roots) != 1 {
		t.Fatalf("partial result = %+v", res)
	}
	checkSums(t, res.Roots[0])
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
