// Package scan walks directories and builds a size-annotated tree.
package scan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/charlievieth/fastwalk"
)

const maxErrors = 100

// Node is a file or directory. Dir sizes are recursive disk usage.
// Nodes returned by Snapshot or Result are copies, safe to read freely.
type Node struct {
	Name      string
	Path      string
	IsDir     bool
	Size      int64 // bytes on disk (st_blocks*512)
	Files     int64 // files at or under this node, including those below FileFloor
	FileBytes int64 // their usage (Size also counts dirs)
	ModTime   time.Time
	Parent    *Node
	Children  []*Node
}

// Options control a scan.
type Options struct {
	FileFloor int64    // files smaller than this count toward parent size but get no Node
	Exclude   []string // absolute paths to skip
}

// Progress is a live snapshot of scan counters.
type Progress struct {
	Files, Dirs, Bytes, Errors int64
	Current                    string
}

// Result is a finished (or cancelled) scan.
type Result struct {
	Roots                          []*Node
	Files, Dirs, Bytes, ErrorCount int64
	Errors                         []error // first maxErrors errors
}

type inode struct{ dev, ino uint64 }

// live is the scanner's internal node; all fields after creation are guarded by Job.mu.
type live struct {
	name, path string
	isDir      bool
	size       int64 // recursive; updated on discovery so partial totals are consistent
	files      int64 // recursive count of files seen, including those below FileFloor
	fileBytes  int64 // recursive usage of those files (size also counts dirs)
	mod        time.Time
	parent     *live
	children   []*live
}

// Job is a scan running in the background.
type Job struct {
	opts     Options
	excluded map[string]bool

	files, dirs, bytes, errCount atomic.Int64
	current                      atomic.Pointer[string]

	mu     sync.Mutex
	errs   []error
	roots  []*live
	nodes  map[string]*live // dir path → node, per root
	linked map[inode]bool   // hard links seen, per root

	done chan struct{}
	res  *Result
	err  error
}

// DefaultExcludes returns paths skipped by default on macOS.
func DefaultExcludes(home string) []string {
	return []string{
		"/System",
		"/Volumes",
		"/dev",
		"/private/var/vm",
		filepath.Join(home, "Library", "Mobile Documents"), // iCloud Drive
		filepath.Join(home, "Library", "CloudStorage"),     // Dropbox, OneDrive, Google Drive
		filepath.Join(home, ".Trash"),
	}
}

// Start scans roots in the background. Permission and read errors are
// collected, not fatal; a bad root or cancellation ends the scan early.
func Start(ctx context.Context, roots []string, opts Options) *Job {
	s := &Job{opts: opts, excluded: map[string]bool{}, done: make(chan struct{})}
	for _, e := range opts.Exclude {
		// Roots are symlink-resolved, so excludes must be too (/tmp → /private/tmp).
		if real, err := filepath.EvalSymlinks(e); err == nil {
			e = real
		}
		s.excluded[filepath.Clean(e)] = true
	}
	go s.run(ctx, roots)
	return s
}

// Scan runs a scan to completion. Progress snapshots are sent non-blocking to
// progress (may be nil) roughly 10×/s. On error it returns no result.
func Scan(ctx context.Context, roots []string, opts Options, progress chan<- Progress) (*Result, error) {
	s := Start(ctx, roots, opts)
	if progress != nil {
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
	loop:
		for {
			select {
			case <-s.Done():
				break loop
			case <-t.C:
				send(progress, s.Progress())
			}
		}
		send(progress, s.Progress())
	}
	res, err := s.Result()
	if err != nil {
		return nil, err
	}
	return res, nil
}

func send(ch chan<- Progress, p Progress) {
	select {
	case ch <- p:
	default:
	}
}

// Restore rebuilds a finished job from a tree taken with Snapshot (e.g. saved
// between runs) and its totals, so it can be filtered, snapshotted and pruned
// with Remove like a fresh scan. The tree is copied.
func Restore(roots []*Node, totals Progress, floor int64) *Job {
	s := &Job{opts: Options{FileFloor: floor}, done: make(chan struct{})}
	var build func(n *Node, parent *live) *live
	build = func(n *Node, parent *live) *live {
		l := &live{name: n.Name, path: n.Path, isDir: n.IsDir, size: n.Size, files: n.Files, fileBytes: n.FileBytes, mod: n.ModTime, parent: parent}
		for _, c := range n.Children {
			l.children = append(l.children, build(c, l))
		}
		return l
	}
	for _, r := range roots {
		s.roots = append(s.roots, build(r, nil))
	}
	s.files.Store(totals.Files)
	s.dirs.Store(totals.Dirs)
	s.bytes.Store(totals.Bytes)
	s.errCount.Store(totals.Errors)
	s.res = &Result{Roots: s.Snapshot(floor), Files: totals.Files, Dirs: totals.Dirs, Bytes: totals.Bytes, ErrorCount: totals.Errors}
	close(s.done)
	return s
}

// Done is closed when the scan finishes, fails or is cancelled.
func (s *Job) Done() <-chan struct{} { return s.done }

// Result waits for the scan to end. After cancellation it returns the partial
// result together with the context error.
func (s *Job) Result() (*Result, error) {
	<-s.done
	return s.res, s.err
}

// Progress returns the current counters.
func (s *Job) Progress() Progress {
	p := Progress{Files: s.files.Load(), Dirs: s.dirs.Load(), Bytes: s.bytes.Load(), Errors: s.errCount.Load()}
	if c := s.current.Load(); c != nil {
		p.Current = *c
	}
	return p
}

// Snapshot copies the tree so far, keeping roots plus nodes with Size >= min.
// Cost is proportional to the nodes kept, not the whole scan.
func (s *Job) Snapshot(min int64) []*Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Node, 0, len(s.roots))
	for _, r := range s.roots {
		out = append(out, copyNode(r, nil, min))
	}
	return out
}

func copyNode(l *live, parent *Node, min int64) *Node {
	n := &Node{Name: l.name, Path: l.path, IsDir: l.isDir, Size: l.size, Files: l.files, FileBytes: l.fileBytes, ModTime: l.mod, Parent: parent}
	for _, c := range l.children {
		if c.size >= min {
			n.Children = append(n.Children, copyNode(c, n, min))
		}
	}
	return n
}

func (s *Job) run(ctx context.Context, roots []string) {
	defer close(s.done)
	s.err = ctx.Err()
	for _, root := range roots {
		if s.err != nil {
			break
		}
		s.err = s.scanRoot(ctx, root)
	}
	s.mu.Lock()
	errs := append([]error(nil), s.errs...)
	s.mu.Unlock()
	s.res = &Result{
		Roots:      s.Snapshot(s.opts.FileFloor),
		Files:      s.files.Load(),
		Dirs:       s.dirs.Load(),
		Bytes:      s.bytes.Load(),
		ErrorCount: s.errCount.Load(),
		Errors:     errs,
	}
}

func (s *Job) scanRoot(ctx context.Context, root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	info, err := os.Lstat(real)
	if err != nil {
		return err
	}
	st := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() {
		s.files.Add(1)
		s.bytes.Add(st.Blocks * 512)
		s.mu.Lock()
		usage := st.Blocks * 512
		s.roots = append(s.roots, &live{name: real, path: real, size: usage, files: 1, fileBytes: usage, mod: info.ModTime()})
		s.mu.Unlock()
		return nil
	}

	s.mu.Lock()
	s.nodes = map[string]*live{}
	s.linked = map[inode]bool{}
	s.mu.Unlock()
	if err := fastwalk.Walk(&fastwalk.Config{}, real, s.walkFn(ctx, real, rootDevices(real, st))); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nodes[real] == nil {
		return fmt.Errorf("%s: could not read", real)
	}
	return nil
}

// rootDevices lists the filesystems a walk from root may enter. On macOS 10.15+
// "/" is the sealed system volume and /Users, /Applications etc. are firmlinks
// into the Data volume, so scanning "/" must include that device too.
// (/System, which holds /System/Volumes/Data, is a default exclude, so nothing
// is counted twice.)
func rootDevices(root string, st *syscall.Stat_t) map[uint64]bool {
	devs := map[uint64]bool{uint64(st.Dev): true}
	if root == "/" {
		if info, err := os.Lstat("/System/Volumes/Data"); err == nil {
			devs[uint64(info.Sys().(*syscall.Stat_t).Dev)] = true
		}
	}
	return devs
}

func (s *Job) walkFn(ctx context.Context, root string, devs map[uint64]bool) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		path = filepath.Clean(path)
		if err != nil {
			if path == root {
				return err // an unreadable root would otherwise look empty
			}
			s.addErr(err)
			return nil
		}
		if path != root && s.excluded[path] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// A dir without a node would orphan its subtree, so skip it.
		skip := func() error {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			s.addErr(err)
			return skip()
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return skip()
		}
		usage := st.Blocks * 512
		if d.IsDir() {
			if !devs[uint64(st.Dev)] {
				return fs.SkipDir // different filesystem (mount point)
			}
			if !s.addDir(path, root, info.ModTime(), usage) {
				return fs.SkipDir
			}
			return nil
		}
		if st.Nlink > 1 && !s.firstLink(inode{uint64(st.Dev), st.Ino}) {
			return nil
		}
		s.addFile(path, info.ModTime(), usage)
		return nil
	}
}

// grow adds usage to n and all its ancestors. Caller holds s.mu.
func grow(n *live, usage int64) {
	for ; n != nil; n = n.parent {
		n.size += usage
	}
}

// addDir adds a dir under its parent and reports whether it was attached. It
// isn't when its parent is gone (Remove'd mid-scan, or skipped); the caller
// should then skip its subtree. Counters only include attached entries, so
// the totals always match the tree.
func (s *Job) addDir(path, root string, mod time.Time, usage int64) bool {
	s.current.Store(&path)

	n := &live{name: filepath.Base(path), path: path, isDir: true, mod: mod}
	s.mu.Lock()
	defer s.mu.Unlock()
	if path == root {
		n.name = path
		s.roots = append(s.roots, n)
	} else if p := s.nodes[filepath.Dir(path)]; p != nil {
		n.parent = p
		p.children = append(p.children, n)
	} else {
		return false
	}
	s.nodes[path] = n
	s.dirs.Add(1)
	grow(n, usage)
	return true
}

func (s *Job) addFile(path string, mod time.Time, usage int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.nodes[filepath.Dir(path)]
	if p == nil {
		return
	}
	s.files.Add(1)
	s.bytes.Add(usage)
	if usage >= s.opts.FileFloor {
		p.children = append(p.children, &live{name: filepath.Base(path), path: path, size: usage, files: 1, fileBytes: usage, mod: mod, parent: p})
	}
	for n := p; n != nil; n = n.parent {
		n.size += usage
		n.files++
		n.fileBytes += usage
	}
}

// Remove drops paths and everything under them, e.g. after they were deleted
// or moved to the Bin, taking their usage off every ancestor and the totals.
// Roots and paths not in the tree are ignored.
func (s *Job) Remove(paths ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range paths {
		for _, r := range s.roots { // overlapping roots may each hold it
			n := find(r, filepath.Clean(p))
			if n == nil {
				continue
			}
			parent := n.parent
			parent.children = slices.DeleteFunc(parent.children, func(c *live) bool { return c == n })
			for a := parent; a != nil; a = a.parent {
				a.size -= n.size
				a.files -= n.files
				a.fileBytes -= n.fileBytes
			}
			s.files.Add(-n.files)
			s.bytes.Add(-n.fileBytes)
			s.dirs.Add(-s.forget(n))
		}
	}
}

// find returns the node at path strictly below root r, or nil.
func find(r *live, path string) *live {
	rel, err := filepath.Rel(r.path, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil
	}
	n := r
	for _, name := range strings.Split(rel, "/") {
		i := slices.IndexFunc(n.children, func(c *live) bool { return c.name == name })
		if i < 0 {
			return nil
		}
		n = n.children[i]
	}
	return n
}

// forget unindexes n's dirs and returns how many there were. Caller holds s.mu.
func (s *Job) forget(n *live) int64 {
	if !n.isDir {
		return 0
	}
	if s.nodes[n.path] == n {
		delete(s.nodes, n.path)
	}
	dirs := int64(1)
	for _, c := range n.children {
		dirs += s.forget(c)
	}
	return dirs
}

func (s *Job) firstLink(k inode) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.linked[k] {
		return false
	}
	s.linked[k] = true
	return true
}

func (s *Job) addErr(err error) {
	s.errCount.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.errs) < maxErrors {
		s.errs = append(s.errs, err)
	}
}
