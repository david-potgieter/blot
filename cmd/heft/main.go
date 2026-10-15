// Command heft finds big files and folders on macOS.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/david-potgieter/heft/internal/scan"
	"github.com/david-potgieter/heft/internal/ui"
)

var version = "dev" // set by goreleaser

func main() {
	minSize := flag.String("min", "100MB", "minimum size, e.g. 500MB, 1GiB")
	typ := flag.String("type", "all", "what to show: all, dirs, files")
	noForm := flag.Bool("no-form", false, "skip the form and scan immediately")
	showVersion := flag.Bool("version", false, "print version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: heft [paths...] [flags]\n\nFind big files and folders.\n\n")
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
		fmt.Println("heft", version)
		return
	}
	if len(roots) == 0 {
		roots = []string{"~"}
	}

	home, _ := os.UserHomeDir()
	app, err := ui.New(ui.Config{
		Roots:    strings.Join(roots, ", "),
		Min:      *minSize,
		Type:     *typ,
		SkipForm: *noForm,
		Excludes: scan.DefaultExcludes(home),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "heft:", err)
		os.Exit(2)
	}
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "heft:", err)
		os.Exit(1)
	}
}
