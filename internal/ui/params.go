package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/david-potgieter/blot/internal/size"
	"github.com/david-potgieter/blot/internal/tree"
)

// Params is a validated search.
type Params struct {
	Roots []string
	Min   int64
	Type  tree.Type
}

// formValues is the raw form input.
type formValues struct {
	roots string
	min   string
	typ   tree.Type
}

func (v *formValues) params() (Params, error) {
	roots, err := parseRoots(v.roots)
	if err != nil {
		return Params{}, err
	}
	minSize, err := size.Parse(v.min)
	if err != nil {
		return Params{}, err
	}
	return Params{Roots: roots, Min: minSize, Type: v.typ}, nil
}

// parseRoots splits comma-separated paths, expands ~/, makes them absolute,
// drops exact duplicates and checks they exist. Errors name the path as typed.
func parseRoots(s string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, typed := range strings.Split(s, ",") {
		typed = strings.TrimSpace(typed)
		if typed == "" {
			continue
		}
		p := typed
		if p == "~" || strings.HasPrefix(p, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("%s: can't find home folder", typed)
			}
			p = filepath.Join(home, p[1:])
		}
		p, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", typed, err)
		}
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s: not found", typed)
		} else if err != nil {
			return nil, fmt.Errorf("%s: %w", typed, err)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("enter at least one path")
	}
	return out, nil
}

func tildify(p, home string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}
