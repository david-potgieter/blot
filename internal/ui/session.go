package ui

import (
	"compress/gzip"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/david-potgieter/heft/internal/scan"
	"github.com/david-potgieter/heft/internal/tree"
)

// sessionVersion changes whenever the saved format does; older files are ignored.
const sessionVersion = 1

// session is the whole view, saved on quit and restored on the next plain start.
type session struct {
	Version int

	Roots, Min string // form, as typed
	Type       tree.Type
	Field      field
	Focus      focus

	// Results, when there were any.
	Scanned   bool
	ScannedAt time.Time
	Params    Params
	Floor     int64
	Cancelled bool
	Totals    scan.Progress
	Nodes     []savedNode
	Sort      tree.SortBy
	Expanded  map[string]bool
	Cursor    string
	Offset    int
}

// savedNode is a scan.Node in preorder; Parent indexes an earlier node, or is
// -1 for a root. Paths are rebuilt from names to keep the file small.
type savedNode struct {
	Name                   string
	Dir                    bool
	Size, Files, FileBytes int64
	Mod                    time.Time
	Parent                 int
}

func flatten(roots []*scan.Node) []savedNode {
	var out []savedNode
	var walk func(n *scan.Node, parent int)
	walk = func(n *scan.Node, parent int) {
		i := len(out)
		out = append(out, savedNode{n.Name, n.IsDir, n.Size, n.Files, n.FileBytes, n.ModTime, parent})
		for _, c := range n.Children {
			walk(c, i)
		}
	}
	for _, r := range roots {
		walk(r, -1)
	}
	return out
}

func unflatten(saved []savedNode) ([]*scan.Node, error) {
	nodes := make([]*scan.Node, len(saved))
	var roots []*scan.Node
	for i, s := range saved {
		n := &scan.Node{Name: s.Name, Path: s.Name, IsDir: s.Dir, Size: s.Size, Files: s.Files, FileBytes: s.FileBytes, ModTime: s.Mod}
		switch {
		case s.Parent == -1:
			roots = append(roots, n)
		case s.Parent >= 0 && s.Parent < i:
			p := nodes[s.Parent]
			n.Path, n.Parent = filepath.Join(p.Path, s.Name), p
			p.Children = append(p.Children, n)
		default:
			return nil, fmt.Errorf("session: node %d has bad parent %d", i, s.Parent)
		}
		nodes[i] = n
	}
	return roots, nil
}

// writeSession saves s atomically, so a crash never leaves half a file.
func writeSession(path string, s *session) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // no-op once renamed
	gz := gzip.NewWriter(f)
	err = errors.Join(gob.NewEncoder(gz).Encode(s), gz.Close(), f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func readSession(path string) (*session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var s session
	if err := gob.NewDecoder(gz).Decode(&s); err != nil {
		return nil, err
	}
	if s.Version != sessionVersion {
		return nil, fmt.Errorf("session: version %d, want %d", s.Version, sessionVersion)
	}
	return &s, nil
}
