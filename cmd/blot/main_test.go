package main

import (
	"path/filepath"
	"testing"

	"github.com/david-potgieter/blot/internal/queries"
)

// withQueries points the default queries file at a temp dir and saves qs.
func withQueries(t *testing.T, qs ...queries.Query) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	for _, q := range qs {
		if err := queries.Save(q.Label, q); err != nil {
			t.Fatal(err)
		}
	}
}

var nodeModules = queries.Query{Label: "node_modules", Roots: "~/code", Min: "50MB", Show: "dirs", Name: "node_modules"}

func TestQueryFillsConfigAndSkipsForm(t *testing.T) {
	withQueries(t, nodeModules)
	cfg, _, err := parseArgs([]string{"-query", "node_modules"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Roots != "~/code" || cfg.Min != "50MB" || cfg.Type != "dirs" || cfg.Name != "node_modules" || !cfg.SkipForm || cfg.Restore {
		t.Fatalf("got %+v", cfg)
	}
}

func TestUnknownQueryListsSavedLabels(t *testing.T) {
	withQueries(t, nodeModules, queries.Query{Label: "target", Min: "1GB"})
	_, _, err := parseArgs([]string{"-query", "unknown"})
	want := "query 'unknown' not found; available: node_modules, target"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestFlagsOverrideQueryForThisRunOnly(t *testing.T) {
	withQueries(t, nodeModules)
	cfg, _, err := parseArgs([]string{"-query", "node_modules", "-min", "0", "-name", "target"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Min != "0" || cfg.Name != "target" || cfg.Type != "dirs" {
		t.Fatalf("got %+v", cfg)
	}
	if saved, _ := queries.Load("node_modules"); saved != nodeModules {
		t.Fatalf("saved query changed: %+v", saved)
	}
}
