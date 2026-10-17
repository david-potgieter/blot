// Command blot finds big files and folders on macOS.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/david-potgieter/blot/internal/queries"
	"github.com/david-potgieter/blot/internal/scan"
	"github.com/david-potgieter/blot/internal/ui"
)

var version = "dev" // set by goreleaser

func main() {
	cfg, showVersion, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "blot:", err)
		os.Exit(2)
	}
	if showVersion {
		// go install builds aren't stamped by goreleaser; use the module version.
		if bi, ok := debug.ReadBuildInfo(); ok && version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
		fmt.Println("blot", version)
		return
	}

	home, _ := os.UserHomeDir()
	cfg.Excludes = scan.DefaultExcludes(home)
	if cache, err := os.UserCacheDir(); err == nil {
		cfg.Session = filepath.Join(cache, "blot", "session.gob.gz")
	}
	app, err := ui.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "blot:", err)
		os.Exit(2)
	}
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "blot:", err)
		os.Exit(1)
	}
}

// parseArgs turns command-line args into a ui.Config, filling from a saved
// -query first; explicit paths and flags override it for this run only.
func parseArgs(args []string) (ui.Config, bool, error) {
	fs := flag.NewFlagSet("blot", flag.ExitOnError)
	minSize := fs.String("min", "100MB", "minimum size, e.g. 500MB, 1GiB")
	typ := fs.String("type", "all", "what to show: all, dirs, files")
	name := fs.String("name", "", "only names matching these comma-separated globs, e.g. node_modules,.next")
	query := fs.String("query", "", "load a saved query by label and scan immediately")
	noForm := fs.Bool("no-form", false, "skip the form and scan immediately")
	showVersion := fs.Bool("version", false, "print version")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: blot [paths...] [flags]\n\nFind big files and folders.\n\n")
		fs.PrintDefaults()
	}

	// Allow flags before and after paths.
	var roots []string
	rest := args
	for {
		_ = fs.Parse(rest)
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		roots = append(roots, rest[0])
		rest = rest[1:]
	}

	cfg := ui.Config{
		Roots:    "~",
		Min:      *minSize,
		Type:     *typ,
		Name:     *name,
		SkipForm: *noForm,
		// A plain `blot` picks up where the last run left off; any paths or flags start fresh.
		Restore: len(args) == 0,
	}
	if *query != "" {
		q, err := loadQuery(*query)
		if err != nil {
			return ui.Config{}, false, err
		}
		cfg.Roots, cfg.Min, cfg.Type, cfg.Name, cfg.SkipForm = q.Roots, q.Min, q.Show, q.Name, true
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		if set["min"] {
			cfg.Min = *minSize
		}
		if set["type"] {
			cfg.Type = *typ
		}
		if set["name"] {
			cfg.Name = *name
		}
	}
	if len(roots) > 0 {
		cfg.Roots = ui.JoinRoots(roots) // escapes commas inside paths
	}
	return cfg, *showVersion, nil
}

// loadQuery returns the saved query, or an error listing the saved labels.
func loadQuery(label string) (queries.Query, error) {
	labels, err := queries.List()
	if err != nil {
		return queries.Query{}, err
	}
	if !slices.Contains(labels, label) {
		return queries.Query{}, fmt.Errorf("query '%s' not found; available: %s", label, strings.Join(labels, ", "))
	}
	return queries.Load(label)
}
