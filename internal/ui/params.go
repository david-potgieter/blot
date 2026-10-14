package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/david-potgieter/heft/internal/size"
	"github.com/david-potgieter/heft/internal/tree"
)

// Params is a validated search.
type Params struct {
	Roots []string
	Min   int64
	Type  tree.Type
}

func (p Params) summary() string {
	home, _ := os.UserHomeDir()
	roots := make([]string, len(p.Roots))
	for i, r := range p.Roots {
		roots[i] = tildify(r, home)
	}
	return fmt.Sprintf("%s · ≥ %s · %s", strings.Join(roots, ", "), size.Format(p.Min), p.Type)
}

// formValues backs the huh form fields (huh needs stable pointers).
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
	min, err := size.Parse(v.min)
	if err != nil {
		return Params{}, err
	}
	return Params{Roots: roots, Min: min, Type: v.typ}, nil
}

// parseRoots splits comma-separated paths, expands ~ and checks they exist.
func parseRoots(s string) ([]string, error) {
	home, _ := os.UserHomeDir()
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "~" {
			p = home
		} else if strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("%s: not found", p)
		}
		out = append(out, filepath.Clean(p))
	}
	if len(out) == 0 {
		return nil, errors.New("enter at least one folder")
	}
	return out, nil
}

func tildify(p, home string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}
