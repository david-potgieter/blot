// Package ui is heft's Bubble Tea interface.
package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"

	"github.com/david-potgieter/heft/internal/scan"
	"github.com/david-potgieter/heft/internal/size"
	"github.com/david-potgieter/heft/internal/tree"
)

const (
	formWidth    = 34        // left pane, including border and padding
	narrowWidth  = 90        // below this only the focused pane is shown
	maxFloor     = 1_000_000 // scans keep files down to min(min size, 1MB)
	tickInterval = 250 * time.Millisecond
)

type focus int

const (
	focusForm focus = iota
	focusResults
)

// Config holds initial form values (from CLI flags).
type Config struct {
	Roots    string // comma-separated
	Min      string // e.g. "500MB"
	Type     string // all | dirs | files
	SkipForm bool   // start scanning immediately
	Excludes []string
}

type (
	tickMsg    struct{ id int }
	jobDoneMsg struct{ id int }
)

// App is the root model: search form on the left, live results on the right.
type App struct {
	cfg    Config
	form   formModel
	tree   treeModel
	focus  focus
	help   help.Model
	styles styles
	isDark bool

	job       *scan.Job
	jobID     int
	cancel    context.CancelFunc
	jobParams Params // what the current job is scanning
	floor     int64  // smallest file size the current job keeps
	scanning  bool
	cancelled bool
	snapshot  []*scan.Node
	progress  scan.Progress
	err       error // scan failed outright (e.g. a root became unreadable)
	filter    tree.Filter

	width, height int
	initCmd       tea.Cmd
}

// New builds the app. With SkipForm it validates cfg and starts scanning.
func New(cfg Config) (*App, error) {
	typ, err := tree.ParseType(cfg.Type)
	if err != nil {
		return nil, err
	}
	st := newStyles(true)
	a := &App{
		cfg:    cfg,
		form:   newFormModel(formValues{roots: cfg.Roots, min: cfg.Min, typ: typ}, st),
		tree:   newTreeModel(st),
		help:   help.New(),
		styles: st,
		isDark: true,
	}
	if cfg.SkipForm {
		v := a.form.values()
		if _, err := v.params(); err != nil {
			return nil, err
		}
		a.initCmd = a.startScan()
		return a, nil
	}
	a.initCmd = a.form.setFocused(true)
	return a, nil
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, a.initCmd)
}

// startScan cancels any running scan and starts one from the form values.
func (a *App) startScan() tea.Cmd {
	v := a.form.values()
	p, err := v.params()
	if err != nil {
		return nil // the form shows the error inline
	}
	a.stopScan()
	ctx, cancel := context.WithCancel(context.Background())
	a.floor = min(p.Min, maxFloor)
	a.job = scan.Start(ctx, p.Roots, scan.Options{FileFloor: a.floor, Exclude: a.cfg.Excludes})
	a.jobID++
	a.cancel = cancel
	a.jobParams = p
	a.scanning, a.cancelled = true, false
	a.form.scanning = true
	a.snapshot, a.progress, a.err = nil, scan.Progress{}, nil
	a.tree.setTree(nil) // new search: fresh expansion state
	a.rebuild()
	return tea.Batch(a.setFocus(focusResults), tick(a.jobID), waitJob(a.job, a.jobID))
}

// stopScan cancels the running scan; its jobDoneMsg finalises the partial result.
func (a *App) stopScan() {
	if a.scanning && a.cancel != nil {
		a.cancel()
		a.cancelled = true
	}
}

func tick(id int) tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tickMsg{id} })
}

func waitJob(job *scan.Job, id int) tea.Cmd {
	return func() tea.Msg {
		<-job.Done()
		return jobDoneMsg{id}
	}
}

func (a *App) setFocus(f focus) tea.Cmd {
	a.focus = f
	return a.form.setFocused(f == focusForm)
}

// currentFilter applies the form's min size and type to existing results.
// A min below the scan floor can't be shown without rescanning, so it is
// clamped to the floor (and the header asks for a rescan).
func (a *App) currentFilter() tree.Filter {
	f := tree.Filter{Min: a.filter.Min, Type: a.form.typ}
	if m, err := size.Parse(a.form.min.Value()); err == nil {
		f.Min = max(m, a.floor)
	}
	return f
}

// rebuild re-filters the latest snapshot, keeping sort, expansion and cursor.
func (a *App) rebuild() {
	if a.job == nil {
		return
	}
	a.filter = a.currentFilter()
	sort, expanded := tree.BySize, map[string]bool(nil)
	if a.tree.tree != nil {
		sort, expanded = a.tree.tree.Sort(), a.tree.tree.ExpandedPaths()
	}
	a.tree.setTree(tree.Rebuild(a.snapshot, a.filter, sort, expanded))
}

// stale reports whether the form asks for something the current scan can't show.
func (a *App) stale() bool {
	if a.job == nil {
		return false
	}
	roots, err := parseRoots(a.form.roots.Value())
	if err == nil && !slices.Equal(roots, a.jobParams.Roots) {
		return true
	}
	m, err := size.Parse(a.form.min.Value())
	return err == nil && m < a.floor
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.layout()
		return a, nil

	case tea.BackgroundColorMsg:
		a.isDark = msg.IsDark()
		a.styles = newStyles(a.isDark)
		a.form.styles, a.tree.styles = a.styles, a.styles
		a.form.roots.SetStyles(textinput.DefaultStyles(a.isDark))
		a.form.min.SetStyles(textinput.DefaultStyles(a.isDark))
		return a, nil

	case tickMsg:
		if msg.id != a.jobID || !a.scanning {
			return a, nil
		}
		a.progress = a.job.Progress()
		a.snapshot = a.job.Snapshot(a.floor)
		a.rebuild()
		return a, tick(a.jobID)

	case jobDoneMsg:
		if msg.id != a.jobID {
			return a, nil
		}
		res, err := a.job.Result()
		a.cancel()
		a.scanning = false
		a.form.scanning = false
		if err != nil && !errors.Is(err, context.Canceled) {
			a.err = err
		}
		if res != nil {
			a.snapshot = res.Roots
			a.progress = scan.Progress{Files: res.Files, Dirs: res.Dirs, Bytes: res.Bytes, Errors: res.ErrorCount}
		}
		a.rebuild()
		return a, nil

	case submitMsg:
		if a.scanning {
			a.stopScan()
			return a, nil
		}
		return a, a.startScan()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			a.stopScan()
			return a, tea.Quit
		}
		if a.focus == focusForm {
			if msg.String() == "esc" && a.job != nil {
				return a, a.setFocus(focusResults)
			}
			var cmd tea.Cmd
			a.form, cmd = a.form.Update(msg)
			a.rebuild() // min/type edits re-filter immediately
			return a, cmd
		}
		switch msg.String() {
		case "tab":
			return a, a.setFocus(focusForm)
		case "q":
			a.stopScan()
			return a, tea.Quit
		case "r":
			return a, a.startScan()
		}
		var cmd tea.Cmd
		a.tree, cmd = a.tree.Update(msg)
		return a, cmd
	}

	// Cursor blink and other input messages.
	var cmd tea.Cmd
	a.form, cmd = a.form.Update(msg)
	return a, cmd
}

// paneHeight is the outer height of both panes (the help line sits below).
func (a *App) paneHeight() int { return max(5, a.height-1) }

// paneWidths returns the outer widths of the form and results panes; 0 hides a pane.
func (a *App) paneWidths() (form, results int) {
	switch {
	case a.width >= narrowWidth:
		return formWidth, a.width - formWidth
	case a.focus == focusForm:
		return a.width, 0
	default:
		return 0, a.width
	}
}

// layout sizes sub-models to the panes. Panes have a 1-cell border and
// 1-cell horizontal padding; results reserve 2 lines for header + gap.
func (a *App) layout() {
	fw, rw := a.paneWidths()
	if fw == 0 {
		fw = formWidth
	}
	a.form.setWidth(fw - 4)
	if rw == 0 {
		rw = a.width
	}
	a.tree.setSize(max(1, rw-4), max(1, a.paneHeight()-2-2))
	a.help.SetWidth(a.width)
}

func (a *App) View() tea.View {
	a.layout() // focus changes alter pane widths in narrow mode
	fw, rw := a.paneWidths()
	h := a.paneHeight()

	var panes []string
	if fw > 0 {
		st := a.styles.pane
		if a.focus == focusForm {
			st = a.styles.focusedPane
		}
		panes = append(panes, st.Width(fw).Height(h).Render(a.styles.title.Render("Search")+"\n\n"+a.form.View()))
	}
	if rw > 0 {
		st := a.styles.pane
		if a.focus == focusResults {
			st = a.styles.focusedPane
		}
		panes = append(panes, st.Width(rw).Height(h).Render(a.resultsView(rw-4)))
	}

	bindings := formHelp()
	if a.focus == focusResults {
		bindings = a.tree.keys.ShortHelp()
	}
	out := lipgloss.JoinHorizontal(lipgloss.Top, panes...) + "\n" + a.help.ShortHelpView(bindings)

	v := tea.NewView(out)
	v.AltScreen = true
	return v
}

// resultsView is the header line plus the tree, or a hint before the first scan.
func (a *App) resultsView(w int) string {
	s := a.styles
	if a.job == nil {
		return s.title.Render("Results") + "\n\n" + s.subtle.Render("Set a search and press enter.")
	}

	status := "done"
	switch {
	case a.scanning:
		status = "scanning"
	case a.cancelled:
		status = "cancelled"
	}
	p := a.progress
	parts := []string{
		s.title.Render("Results"),
		status,
		humanize.Comma(p.Files) + " files",
		size.Format(p.Bytes),
	}
	if a.tree.tree != nil {
		parts = append(parts, fmt.Sprintf("%d matches", a.tree.tree.MatchCount()))
	}
	header := strings.Join(parts, s.subtle.Render(" · "))
	switch {
	case a.err != nil:
		header += " · " + s.err.Render(a.err.Error())
	case a.stale():
		header += " · " + s.err.Render("press Search to rescan")
	case p.Errors > 0:
		header += " · " + s.err.Render(fmt.Sprintf("%d unreadable (Full Disk Access?)", p.Errors))
	}
	header = lipgloss.NewStyle().MaxWidth(w).Render(header)
	return header + "\n\n" + a.tree.View()
}
