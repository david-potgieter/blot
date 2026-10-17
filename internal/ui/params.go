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
	Roots        []string
	Min          int64
	Type         tree.Type
	NamePatterns []string
}

// formValues is the raw form input.
type formValues struct {
	roots string
	min   string
	name  string
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
	patterns, err := parseNamePatterns(v.name)
	if err != nil {
		return Params{}, err
	}
	return Params{Roots: roots, Min: minSize, Type: v.typ, NamePatterns: patterns}, nil
}

// The Folders field separates paths with commas. Inside a path a comma is
// written `\,` and a backslash `\\`, so any path round-trips.

// JoinRoots writes paths as a Folders field value, escaping as needed.
func JoinRoots(paths []string) string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = escapeRoot(p)
	}
	return strings.Join(out, ", ")
}

var rootEscaper = strings.NewReplacer(`\`, `\\`, ",", `\,`)

func escapeRoot(p string) string { return rootEscaper.Replace(p) }

// unescapeRoot turns `\\` into `\` and `\,` into `,`; any other backslash is kept.
func unescapeRoot(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' && i+1 < len(p) && (p[i+1] == '\\' || p[i+1] == ',') {
			i++
		}
		b.WriteByte(p[i])
	}
	return b.String()
}

// separators returns the indexes of the commas that separate paths, i.e. those
// not escaped by a backslash (which itself may be escaped).
func separators(s string) []int {
	var out []int
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++ // the next byte is escaped
		case ',':
			out = append(out, i)
		}
	}
	return out
}

// lastSeparator is the index of the last separating comma in s, or -1.
func lastSeparator(s string) int {
	seps := separators(s)
	if len(seps) == 0 {
		return -1
	}
	return seps[len(seps)-1]
}

// splitRoots splits a Folders field value into paths and unescapes each.
func splitRoots(s string) []string {
	var out []string
	start := 0
	for _, i := range separators(s) {
		out = append(out, unescapeRoot(s[start:i]))
		start = i + 1
	}
	return append(out, unescapeRoot(s[start:]))
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

// parseNamePatterns splits the Name field into lowercase globs, escaped like
// Folders, and checks each is a valid pattern. Errors name the pattern as typed.
func parseNamePatterns(s string) ([]string, error) {
	out := []string{}
	for _, typed := range splitRoots(s) {
		typed = strings.TrimSpace(typed)
		if typed == "" {
			continue
		}
		p := strings.ToLower(typed)
		if _, err := filepath.Match(p, ""); err != nil {
			return nil, fmt.Errorf("%s: invalid pattern", typed)
		}
		out = append(out, p)
	}
	return out, nil
}

func tildify(p, home string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}
