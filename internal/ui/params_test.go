package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/david-potgieter/heft/internal/tree"
)

func TestParseRoots(t *testing.T) {
	home, _ := os.UserHomeDir()
	tmp := t.TempDir()
	got, err := parseRoots(" ~ , " + tmp + ",")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != home || got[1] != tmp {
		t.Errorf("got %v", got)
	}
	if _, err := parseRoots(filepath.Join(tmp, "missing")); err == nil {
		t.Error("expected error for missing path")
	}
	if _, err := parseRoots("  , "); err == nil {
		t.Error("expected error for empty")
	}
}

func TestFormValuesParams(t *testing.T) {
	tmp := t.TempDir()
	v := &formValues{roots: tmp, min: "1MB", typ: tree.Files}
	p, err := v.params()
	if err != nil {
		t.Fatal(err)
	}
	if p.Min != 1_000_000 || p.Type != tree.Files || p.Roots[0] != tmp {
		t.Errorf("params = %+v", p)
	}
	v.min = "lots"
	if _, err := v.params(); err == nil {
		t.Error("expected size error")
	}
}
