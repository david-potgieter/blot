// Command blot finds big files and folders on macOS.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	tea "charm.land/bubbletea/v2"

	"github.com/david-potgieter/blot/internal/scan"
	"github.com/david-potgieter/blot/internal/ui"
)

var version = "dev" // set by goreleaser

func main() {
	minSize := flag.String("min", "100MB", "minimum size, e.g. 500MB, 1GiB")
	typ := flag.String("type", "all", "what to show: all, dirs, files")
	noForm := flag.Bool("no-form", false, "skip the form and scan immediately")
	showVersion := flag.Bool("version", false, "print version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: blot [paths...] [flags]\n\nFind big files and folders.\n\n")
		flag.PrintDefaults()
	}

	// Allow flags before and after paths.
	var roots []string
	args := os.Args[1:]
	for {
		_ = flag.CommandLine.Parse(args)
		args = flag.Args()
		if len(args) == 0 {
			break
		}
		roots = append(roots, args[0])
		args = args[1:]
	}
	if *showVersion {
		// go install builds aren't stamped by goreleaser; use the module version.
		if bi, ok := debug.ReadBuildInfo(); ok && version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = bi.Main.Version
		}
		fmt.Println("blot", version)
		return
	}
	// A plain `blot` picks up where the last run left off; any paths or flags start fresh.
	restore := len(os.Args) == 1
	if len(roots) == 0 {
		roots = []string{"~"}
	}

	home, _ := os.UserHomeDir()
	var session string
	if cache, err := os.UserCacheDir(); err == nil {
		session = filepath.Join(cache, "blot", "session.gob.gz")
	}
	app, err := ui.New(ui.Config{
		Roots:    ui.JoinRoots(roots), // escapes commas inside paths
		Min:      *minSize,
		Type:     *typ,
		SkipForm: *noForm,
		Excludes: scan.DefaultExcludes(home),
		Session:  session,
		Restore:  restore,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "blot:", err)
		os.Exit(2)
	}
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "blot:", err)
		os.Exit(1)
	}
}
