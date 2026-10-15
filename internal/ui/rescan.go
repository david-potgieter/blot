package ui

import (
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/david-potgieter/blot/internal/scan"
)

// fullDiskAccessURL opens System Settings at Privacy & Security → Full Disk Access.
const fullDiskAccessURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"

// rescanDialog asks whether to rescan from the folder under the cursor (a
// file's parent) or everything. "From here" puts the folder in the form, so
// the form always shows what is being scanned; "All" rescans the form.
func (a *App) rescanDialog(n *scan.Node) *dialog {
	here := n.Path
	if !n.IsDir {
		here = filepath.Dir(here)
	}
	return &dialog{
		title:   "Rescan",
		body:    tildify(here, a.tree.home),
		options: []string{"From here", "All"},
		onPick: func(choice int) tea.Cmd {
			if choice == 0 {
				a.form.roots.SetValue(tildify(here, a.tree.home))
				a.form.suggest()
			}
			return a.startScan()
		},
	}
}

// accessDialog offers to open the Full Disk Access settings.
func (a *App) accessDialog() *dialog {
	return &dialog{
		title: "Full Disk Access",
		body: "macOS won't let blot read the Bin.\n\n" +
			"In Settings, add the app blot runs in\n" +
			"(Terminal, iTerm, VS Code…), then quit\n" +
			"and reopen that app.",
		options: []string{"Open Settings", "Not now"},
		onPick: func(choice int) tea.Cmd {
			if choice != 0 {
				return nil
			}
			return func() tea.Msg {
				_ = exec.Command("open", fullDiskAccessURL).Run()
				return nil
			}
		},
	}
}
