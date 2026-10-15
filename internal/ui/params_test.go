package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/david-potgieter/blot/internal/tree"
)

func TestRootsWithCommas(t *testing.T) {
	tmp := t.TempDir()
	odd := filepath.Join(tmp, "Photos, 2024")
	if err := os.Mkdir(odd, 0o755); err != nil {
		t.Fatal(err)
	}
	field := JoinRoots([]string{odd, tmp})
	got, err := parseRoots(field)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != odd || got[1] != tmp {
		t.Errorf("parseRoots(%q) = %q", field, got)
	}
}

func TestJoinSplitRootsRoundTrip(t *testing.T) {
	for _, paths := range [][]string{
		{`foo\`, "bar"},       // trailing backslash must not escape the separator
		{`a\,b`},              // a literal \, stays as is
		{"Photos, 2024", "x"}, // comma inside a path
		{`a\\b`, `c\`},
	} {
		got := splitRoots(JoinRoots(paths))
		for i := range got {
			got[i] = strings.TrimSpace(got[i]) // parseRoots trims around separators
		}
		if !slices.Equal(got, paths) {
			t.Errorf("round trip %q via %q = %q", paths, JoinRoots(paths), got)
		}
	}
}

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

func TestParseRootsNormalizes(t *testing.T) {
	home, _ := os.UserHomeDir()
	tmp := t.TempDir()
	t.Chdir(tmp)
	if err := os.Mkdir("sub", 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := parseRoots("sub, " + tmp + "/sub/, ~/")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(tmp, "sub"), home}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v (relative made absolute, duplicates dropped)", got, want)
	}
	_, err = parseRoots("nope-missing")
	if err == nil || err.Error() != "nope-missing: not found" {
		t.Errorf("err = %v, want message naming the typed path", err)
	}
}

func TestTildify(t *testing.T) {
	cases := map[string]string{"/Users/a": "~", "/Users/a/x": "~/x", "/Users/ab": "/Users/ab", "/tmp": "/tmp"}
	for in, want := range cases {
		if got := tildify(in, "/Users/a"); got != want {
			t.Errorf("tildify(%q) = %q, want %q", in, got, want)
		}
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
