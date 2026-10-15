package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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

// The Folders field separates paths with commas; a comma inside a path is
// written as `\,`.

// JoinRoots writes paths as a Folders field value, escaping their commas.
func JoinRoots(paths []string) string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = escapeRoot(p)
	}
	return strings.Join(out, ", ")
}

func escapeRoot(p string) string { return strings.ReplaceAll(p, ",", `\,`) }

func unescapeRoot(p string) string { return strings.ReplaceAll(p, `\,`, ",") }

// lastSeparator is the index of the last unescaped comma in s, or -1.
func lastSeparator(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ',' && (i == 0 || s[i-1] != '\\') {
			return i
		}
	}
	return -1
}

// splitRoots splits a Folders field value on unescaped commas and unescapes each path.
func splitRoots(s string) []string {
	var out []string
	for {
		i := lastSeparator(s)
		out = append(out, unescapeRoot(s[i+1:]))
		if i < 0 {
			break
		}
		s = s[:i]
	}
	slices.Reverse(out)
	return out
}

// parseRoots splits comma-separated paths, expands ~/, makes them absolute,
// drops exact duplicates and checks they exist. Errors name the path as typed.
func parseRoots(s string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, typed := range splitRoots(s) {
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
