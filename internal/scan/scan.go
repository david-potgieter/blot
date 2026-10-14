// Package scan walks directories and builds a size-annotated tree.
package scan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/charlievieth/fastwalk"
)

const maxErrors = 100

// Node is a file or directory. Dir sizes are recursive disk usage.
type Node struct {
	Name     string
	Path     string
	IsDir    bool
	Size     int64 // bytes on disk (st_blocks*512)
	ModTime  time.Time
	Parent   *Node
	Children []*Node // dirs: all subdirs + files with Size >= Options.FileFloor
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

// Result is a finished scan.
type Result struct {
	Roots                          []*Node
	Files, Dirs, Bytes, ErrorCount int64
	Errors                         []error // first maxErrors errors
}

type inode struct{ dev, ino uint64 }

type scanner struct {
	opts     Options
	excluded map[string]bool

	files, dirs, bytes, errCount atomic.Int64
	current                      atomic.Pointer[string]

	mu     sync.Mutex
	errs   []error
	nodes  map[string]*Node // dir path → node, per root
	linked map[inode]bool   // hard links seen, per root
}

// DefaultExcludes returns paths skipped by default on macOS.
func DefaultExcludes(home string) []string {
	return []string{
		"/System",
		"/Volumes",
		"/dev",
		"/private/var/vm",
		filepath.Join(home, "Library", "Mobile Documents"),
		filepath.Join(home, ".Trash"),
	}
}

// Scan walks each root and returns its tree. Progress snapshots are sent
// non-blocking to progress (may be nil) roughly 10×/s. Permission and read
// errors are collected, not returned; only a bad root or cancellation fails.
func Scan(ctx context.Context, roots []string, opts Options, progress chan<- Progress) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s := &scanner{opts: opts, excluded: map[string]bool{}}
	for _, e := range opts.Exclude {
		// Roots are symlink-resolved, so excludes must be too (/tmp → /private/tmp).
		if real, err := filepath.EvalSymlinks(e); err == nil {
			e = real
		}
		s.excluded[filepath.Clean(e)] = true
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	if progress != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(100 * time.Millisecond)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					s.emit(progress)
				}
			}
		}()
	}

	res := &Result{}
	var err error
	for _, root := range roots {
		var n *Node
		if n, err = s.scanRoot(ctx, root); err != nil {
			break
		}
		res.Roots = append(res.Roots, n)
	}
	close(stop)
	wg.Wait()
	if err != nil {
		return nil, err
	}
	s.emit(progress)

	res.Files, res.Dirs, res.Bytes, res.ErrorCount = s.files.Load(), s.dirs.Load(), s.bytes.Load(), s.errCount.Load()
	res.Errors = s.errs
	return res, nil
}

func (s *scanner) scanRoot(ctx context.Context, root string) (*Node, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(real)
	if err != nil {
		return nil, err
	}
	st := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() {
		n := &Node{Name: real, Path: real, Size: st.Blocks * 512, ModTime: info.ModTime()}
		s.files.Add(1)
		s.bytes.Add(n.Size)
		return n, nil
	}

	s.nodes = map[string]*Node{}
	s.linked = map[inode]bool{}
	if err := fastwalk.Walk(&fastwalk.Config{}, real, s.walkFn(ctx, real, uint64(st.Dev))); err != nil {
		return nil, err
	}
	n := s.nodes[real]
	if n == nil {
		return nil, fmt.Errorf("%s: could not read", real)
	}
	sumDirs(n)
	return n, nil
}

func (s *scanner) walkFn(ctx context.Context, root string, dev uint64) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			s.addErr(err)
			return nil
		}
		path = filepath.Clean(path)
		if path != root && s.excluded[path] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// A dir without a Node would orphan its subtree, so skip it.
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
			if uint64(st.Dev) != dev {
				return fs.SkipDir // different filesystem (mount point)
			}
			s.addDir(path, root, info.ModTime(), usage)
			return nil
		}
		if st.Nlink > 1 && !s.firstLink(inode{uint64(st.Dev), st.Ino}) {
			return nil
		}
		s.addFile(path, info.ModTime(), usage)
		return nil
	}
}

func (s *scanner) addDir(path, root string, mod time.Time, usage int64) {
	n := &Node{Name: filepath.Base(path), Path: path, IsDir: true, Size: usage, ModTime: mod}
	s.dirs.Add(1)
	s.current.Store(&path)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[path] = n
	if path == root {
		n.Name = path
		return
	}
	if p := s.nodes[filepath.Dir(path)]; p != nil {
		n.Parent = p
		p.Children = append(p.Children, n)
	}
}

func (s *scanner) addFile(path string, mod time.Time, usage int64) {
	s.files.Add(1)
	s.bytes.Add(usage)

	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.nodes[filepath.Dir(path)]
	if p == nil {
		return
	}
	p.Size += usage
	if usage >= s.opts.FileFloor {
		p.Children = append(p.Children, &Node{Name: filepath.Base(path), Path: path, Size: usage, ModTime: mod, Parent: p})
	}
}

func (s *scanner) firstLink(k inode) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.linked[k] {
		return false
	}
	s.linked[k] = true
	return true
}

func (s *scanner) addErr(err error) {
	s.errCount.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.errs) < maxErrors {
		s.errs = append(s.errs, err)
	}
}

func (s *scanner) emit(ch chan<- Progress) {
	if ch == nil {
		return
	}
	p := Progress{Files: s.files.Load(), Dirs: s.dirs.Load(), Bytes: s.bytes.Load(), Errors: s.errCount.Load()}
	if c := s.current.Load(); c != nil {
		p.Current = *c
	}
	select {
	case ch <- p:
	default:
	}
}

// sumDirs adds subdirectory totals into each dir, bottom-up.
func sumDirs(n *Node) int64 {
	for _, c := range n.Children {
		if c.IsDir {
			n.Size += sumDirs(c)
		}
	}
	return n.Size
}
