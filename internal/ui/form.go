package ui

import (
	"charm.land/huh/v2"

	"github.com/david-potgieter/heft/internal/size"
	"github.com/david-potgieter/heft/internal/tree"
)

func newForm(v *formValues) *huh.Form {
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Folders to scan").
			Description("Comma-separated. ~ is your home folder.").
			Value(&v.roots).
			Validate(func(s string) error { _, err := parseRoots(s); return err }),
		huh.NewInput().
			Title("Minimum size").
			Description("e.g. 500MB, 1GB, 2GiB").
			Value(&v.min).
			Validate(func(s string) error { _, err := size.Parse(s); return err }),
		huh.NewSelect[tree.Type]().
			Title("Show").
			Options(
				huh.NewOption("Files and folders", tree.All),
				huh.NewOption("Folders only", tree.Dirs),
				huh.NewOption("Files only", tree.Files),
			).
			Value(&v.typ),
	)).WithShowHelp(true)
}
