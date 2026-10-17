// Package ui is blot's Bubble Tea interface.
package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"

	"github.com/david-potgieter/blot/internal/scan"
	"github.com/david-potgieter/blot/internal/size"
	"github.com/david-potgieter/blot/internal/tree"
)

const (
	formWidth    = 34        // left pane, including border and padding
	narrowWidth  = 90        // below this only the focused pane is shown
	maxFloor     = 1_000_000 // scans keep files down to min(min size, 1MB)
	tickInterval = 250 * time.Millisecond
	minBinHeight = 5 // smallest Bin pane worth showing, including border
)

// focus is a pane; the order is the tab order.
type focus int

const (
	focusForm focus = iota
	focusBin
	focusResults
)

// Config holds initial form values (from CLI flags).
type Config struct {
	Roots    string // comma-separated
	Min      string // e.g. "500MB"
	Type     string // all | dirs | files
	Name     string // comma-separated name globs, e.g. "node_modules, .next"
	SkipForm bool   // start scanning immediately
	Excludes []string
	Bin      string // Trash folder; defaults to ~/.Trash
	Session  string // file the view is saved to on quit; "" disables saving
	Restore  bool   // start from the saved session, if there is a usable one
}

type (
	tickMsg    struct{ id int }
	jobDoneMsg struct{ id int }
)

// App is the root model: search form over the Bin on the left, live results on the right.
type App struct {
	cfg    Config
	form   formModel
	bin    binModel
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
	snapMin   int64 // min the snapshot was taken at; only nodes ≥ it were copied
	progress  scan.Progress
	err       error // scan failed outright (e.g. a root became unreadable)
	filter    tree.Filter
	dialog    *dialog // modal over everything while open
	scannedAt time.Time
	restored  bool // results come from a saved session, not a scan in this run

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
		form:   newFormModel(formValues{roots: cfg.Roots, min: cfg.Min, name: cfg.Name, typ: typ}, st),
		bin:    newBinModel(st),
		tree:   newTreeModel(st),
		help:   help.New(),
		styles: st,
		isDark: true,
	}
	if a.cfg.Bin == "" {
		a.cfg.Bin = filepath.Join(a.tree.home, ".Trash")
	}
	if cfg.Restore && cfg.Session != "" {
		if s, err := readSession(cfg.Session); err == nil {
			a.initCmd = a.restore(s)
			return a, nil
		}
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
	return tea.Batch(tea.RequestBackgroundColor, a.initCmd, a.refreshBin(), pollBin(a.cfg.Bin))
}

// refreshBin re-measures the Bin, keeping the old figures until it's done.
func (a *App) refreshBin() tea.Cmd {
	a.bin.id++
	a.bin.measuring = true
	return measureBin(a.cfg.Bin, a.bin.id)
}

// startScan cancels any running scan and starts one from the form values.
func (a *App) startScan() tea.Cmd {
	v := a.form.values()
	p, err := v.params()
	if err != nil {
		return a.setFocus(focusForm) // the form shows the error inline
	}
	a.stopScan()
	ctx, cancel := context.WithCancel(context.Background())
	a.floor = min(p.Min, maxFloor)
	a.job = scan.Start(ctx, p.Roots, scan.Options{FileFloor: a.floor, Exclude: a.cfg.Excludes})
	a.jobID++
	a.cancel = cancel
	a.jobParams = p
	a.scanning, a.cancelled, a.restored = true, false, false
	a.form.scanning = true
	a.snapshot, a.progress, a.err = nil, scan.Progress{}, nil
	a.tree.setTree(nil) // new search: fresh expansion state
	a.rebuild(true)
	return tea.Batch(a.setFocus(focusResults), tick(a.jobID), waitJob(a.job, a.jobID))
}

// quit stops any scan and saves the view for next time. Saving is best effort:
// failing to save must never stop blot from quitting.
func (a *App) quit() tea.Cmd {
	a.stopScan()
	if a.cfg.Session != "" {
		_ = writeSession(a.cfg.Session, a.session())
	}
	return tea.Quit
}

// session captures the form, results and view as they are now. A scan still
// running is saved as cancelled, with what it has found so far.
func (a *App) session() *session {
	v := a.form.values()
	s := &session{Version: sessionVersion, Roots: v.roots, Min: v.min, Type: v.typ, Field: a.form.focus, Focus: a.focus}
	if a.job == nil {
		return s
	}
	s.Scanned, s.ScannedAt = true, a.scannedAt
	if s.ScannedAt.IsZero() {
		s.ScannedAt = time.Now()
	}
	s.Params, s.Floor, s.Cancelled = a.jobParams, a.floor, a.cancelled
	s.Totals = a.job.Progress()
	s.Totals.Current = ""
	s.Nodes = flatten(a.job.Snapshot(a.floor)) // everything kept, so filters still work
	if t := a.tree.tree; t != nil {
		s.Sort, s.Expanded = t.Sort(), t.ExpandedPaths()
	}
	if n := a.tree.current(); n != nil {
		s.Cursor = n.Path
	}
	s.Offset = a.tree.offset
	return s
}

// restore puts a saved session back on screen, without scanning.
func (a *App) restore(s *session) tea.Cmd {
	a.form.roots.SetValue(s.Roots)
	a.form.suggest()
	a.form.min.SetValue(s.Min)
	a.form.typ, a.form.focus = s.Type, s.Field
	if roots, err := unflatten(s.Nodes); err == nil && s.Scanned {
		a.job = scan.Restore(roots, s.Totals, s.Floor)
		a.jobID++
		a.jobParams, a.floor, a.cancelled = s.Params, s.Floor, s.Cancelled
		a.progress, a.scannedAt, a.restored = s.Totals, s.ScannedAt, true
		a.filter.Min = s.Params.Min                                        // fallback if the saved Min field is invalid
		a.tree.tree = tree.Rebuild(nil, tree.Filter{}, s.Sort, s.Expanded) // carries sort and expansion into rebuild
		a.rebuild(true)
		a.tree.follow(s.Cursor)
		a.tree.wantOffset = s.Offset
	}
	f := s.Focus
	if f == focusResults && a.job == nil {
		f = focusForm
	}
	return a.setFocus(f)
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

// goneMsg lists result paths that no longer exist on disk.
type goneMsg struct {
	id    int
	paths []string
}

// findGone checks everything job kept (down to floor) for paths deleted or
// moved since the scan, skipping anything under a gone dir. The snapshot is
// copied inside the command, off the UI thread; Snapshot locks the job itself.
func findGone(job *scan.Job, floor int64, id int) tea.Cmd {
	return func() tea.Msg {
		roots := job.Snapshot(floor)
		var gone []string
		var walk func(n *scan.Node)
		walk = func(n *scan.Node) {
			for _, c := range n.Children {
				if _, err := os.Lstat(c.Path); errors.Is(err, fs.ErrNotExist) {
					gone = append(gone, c.Path)
				} else if c.IsDir {
					walk(c)
				}
			}
		}
		for _, r := range roots {
			walk(r)
		}
		return goneMsg{id, gone}
	}
}

func (a *App) setFocus(f focus) tea.Cmd {
	a.focus = f
	a.bin.focused = f == focusBin
	a.layout() // in narrow mode the visible pane depends on focus
	return a.form.setFocused(f == focusForm)
}

// cycleFocus moves step panes along the tab order, skipping hidden or empty ones.
func (a *App) cycleFocus(step int) tea.Cmd {
	ps := []focus{focusForm}
	if _, bh := a.leftHeights(); bh > 0 {
		ps = append(ps, focusBin)
	}
	if a.job != nil {
		ps = append(ps, focusResults)
	}
	i := slices.Index(ps, a.focus)
	return a.setFocus(ps[((i+step)%len(ps)+len(ps))%len(ps)])
}

// currentFilter applies the form's min size, name and type to existing results.
// A min below the scan floor can't be shown without rescanning, so it is
// clamped to the floor (and the header asks for a rescan). An unparseable min
// keeps the last valid one, never below the floor, so it can't snapshot everything.
// An invalid name likewise keeps the last valid patterns.
func (a *App) currentFilter() tree.Filter {
	f := tree.Filter{Min: max(a.filter.Min, a.floor), Type: a.form.typ, NamePatterns: a.filter.NamePatterns}
	if m, err := size.Parse(a.form.min.Value()); err == nil {
		f.Min = max(m, a.floor)
	}
	if p, err := parseNamePatterns(a.form.name.Value()); err == nil {
		f.NamePatterns = p
	}
	return f
}

// rebuild re-filters results, keeping sort, expansion and cursor. The job is
// re-snapshotted when fresh (new scan data) or when the min changes, and only
// at the current min, so per-tick cost tracks what is shown, not the floor.
func (a *App) rebuild(fresh bool) {
	if a.job == nil {
		return
	}
	a.filter = a.currentFilter()
	if fresh || a.filter.Min != a.snapMin {
		a.snapshot = a.job.Snapshot(a.filter.Min)
		a.snapMin = a.filter.Min
	}
	sort, expanded := tree.BySize, map[string]bool(nil)
	if a.tree.tree != nil {
		sort, expanded = a.tree.tree.Sort(), a.tree.tree.ExpandedPaths()
	}
	a.tree.setTree(tree.Rebuild(a.snapshot, a.filter, sort, expanded))
}

// refilter applies min/type edits immediately, rebuilding only when they
// change the filter; most form keys (Folders, moving fields) don't.
func (a *App) refilter() {
	if a.job != nil && !a.currentFilter().Equal(a.filter) {
		a.rebuild(false)
	}
}

// stale reports whether the form asks for something the current scan can't show.
func (a *App) stale() bool {
	if a.job == nil {
		return false
	}
	roots, err := a.form.parsedRoots()
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
		if _, bh := a.leftHeights(); bh == 0 && a.focus == focusBin {
			return a, a.setFocus(focusForm) // the Bin no longer fits
		}
		return a, nil

	case tea.BackgroundColorMsg:
		a.isDark = msg.IsDark()
		a.styles = newStyles(a.isDark)
		a.form.styles, a.bin.styles, a.tree.styles = a.styles, a.styles, a.styles
		a.form.roots.SetStyles(textinput.DefaultStyles(a.isDark))
		a.form.min.SetStyles(textinput.DefaultStyles(a.isDark))
		a.form.name.SetStyles(textinput.DefaultStyles(a.isDark))
		return a, nil

	case tickMsg:
		if msg.id != a.jobID || !a.scanning {
			return a, nil
		}
		a.progress = a.job.Progress()
		a.rebuild(true)
		return a, tick(a.jobID)

	case jobDoneMsg:
		if msg.id != a.jobID {
			return a, nil
		}
		res, err := a.job.Result()
		a.cancel()
		a.scanning = false
		a.scannedAt = time.Now()
		a.form.scanning = false
		// Trust the job, not the cancel flag: it may have finished first.
		a.cancelled = errors.Is(err, context.Canceled)
		if err != nil && !a.cancelled {
			a.err = err
		}
		if res != nil {
			a.progress = scan.Progress{Files: res.Files, Dirs: res.Dirs, Bytes: res.Bytes, Errors: res.ErrorCount}
		}
		a.rebuild(true)
		return a, a.refreshBin()

	case binMsg:
		if msg.id != a.bin.id {
			return a, nil
		}
		a.bin.set(msg)
		if a.job == nil {
			return a, nil
		}
		// Whatever went to the Bin is likely in the results too. Check all the
		// scan kept, not just what the filter shows, or lowering the min later
		// would bring trashed items back.
		return a, findGone(a.job, a.floor, a.jobID)

	case goneMsg:
		if msg.id != a.jobID || len(msg.paths) == 0 {
			return a, nil
		}
		a.job.Remove(msg.paths...)
		a.progress = a.job.Progress()
		a.rebuild(true)
		return a, nil

	case binPollMsg:
		// Something was trashed or emptied since the last measurement.
		if !a.bin.measuring && !msg.mod.Equal(a.bin.mod) {
			return a, tea.Batch(a.refreshBin(), pollBin(a.cfg.Bin))
		}
		return a, pollBin(a.cfg.Bin)

	case submitMsg:
		// While scanning the button cancels, unless the form now asks for a
		// different search, in which case it restarts.
		if a.scanning && !a.stale() {
			a.stopScan()
			return a, nil
		}
		return a, a.startScan()

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return a, a.quit()
		}
		if d := a.dialog; d != nil {
			cmd, done := d.update(msg)
			if done && a.dialog == d { // onPick may have opened another
				a.dialog = nil
			}
			return a, cmd
		}
		switch msg.String() {
		case "tab":
			if a.focus == focusForm && a.form.canComplete() {
				break // the form completes the folder
			}
			return a, a.cycleFocus(1)
		case "shift+tab":
			return a, a.cycleFocus(-1)
		}
		if a.focus == focusBin {
			switch msg.String() {
			case "q":
				return a, a.quit()
			case "r":
				return a, a.refreshBin()
			case "enter":
				if a.bin.noAccess() {
					a.dialog = a.accessDialog()
					return a, nil
				}
			}
			var cmd tea.Cmd
			a.bin, cmd = a.bin.Update(msg)
			return a, cmd
		}
		if a.focus == focusForm {
			if msg.String() == "esc" && a.job != nil {
				return a, a.setFocus(focusResults)
			}
			var cmd tea.Cmd
			a.form, cmd = a.form.Update(msg)
			a.form.scanning = a.scanning && !a.stale()
			a.refilter()
			return a, cmd
		}
		switch msg.String() {
		case "q":
			return a, a.quit()
		case "r":
			if n := a.tree.current(); n != nil {
				a.dialog = a.rescanDialog(n)
				return a, nil
			}
			return a, a.startScan()
		}
		var cmd tea.Cmd
		a.tree, cmd = a.tree.Update(msg)
		return a, cmd
	}

	// Cursor blink, paste and other input messages.
	var cmd tea.Cmd
	a.form, cmd = a.form.Update(msg)
	if _, ok := msg.(tea.PasteMsg); ok {
		a.form.scanning = a.scanning && !a.stale()
		a.refilter()
	}
	return a, cmd
}

// paneHeight is the outer height of both panes (the help line sits below).
func (a *App) paneHeight() int { return max(5, a.height-1) }

// leftHeights splits the left column: the form takes what it needs, the Bin
// the rest. When the Bin wouldn't fit, the form takes it all and bin is 0.
func (a *App) leftHeights() (form, bin int) {
	_, form, bin = a.renderLeft()
	return form, bin
}

// leftWidth is the left column's outer width whenever it is shown.
func (a *App) leftWidth() int {
	if a.width >= narrowWidth {
		return formWidth
	}
	return a.width
}

// renderLeft renders the form pane at the width it is drawn and measures it,
// so the height matches the wrapping exactly, and View can use the render
// as is instead of rendering the form a second time.
func (a *App) renderLeft() (formPane string, form, bin int) {
	h, w := a.paneHeight(), a.leftWidth()
	st := a.paneStyle(focusForm).Width(w)
	body := a.formBody()
	pane := st.Render(body)
	if fh := lipgloss.Height(pane); h-fh >= minBinHeight {
		return pane, fh, h - fh
	}
	return st.Height(h).MaxHeight(h).Render(body), h, 0
}

// paneStyle is the border style for pane f, highlighted when it has focus.
func (a *App) paneStyle(f focus) lipgloss.Style {
	if a.focus == f {
		return a.styles.focusedPane
	}
	return a.styles.pane
}

func (a *App) formBody() string { return a.styles.title.Render("Search") + "\n\n" + a.form.View() }

// paneWidths returns the outer widths of the left column and results pane; 0 hides one.
func (a *App) paneWidths() (left, results int) {
	switch {
	case a.width >= narrowWidth:
		return formWidth, a.width - formWidth
	case a.focus != focusResults:
		return a.width, 0
	default:
		return 0, a.width
	}
}

// layout sizes sub-models to the panes. Panes have a 1-cell border and
// 1-cell horizontal padding; results reserve 2 lines for header + gap.
func (a *App) layout() {
	if a.width == 0 || a.height == 0 {
		// No WindowSizeMsg yet (e.g. restoring in New). Sizing now would apply
		// the restored scroll offset to a 1-row list and lose it.
		return
	}
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
	fw, rw := a.paneWidths()
	h := a.paneHeight()

	pane := func(f focus, w, h int) lipgloss.Style {
		return a.paneStyle(f).Width(w).Height(h).MaxHeight(h)
	}
	var panes []string
	if fw > 0 {
		left, _, bh := a.renderLeft()
		if bh > 0 {
			left = lipgloss.JoinVertical(lipgloss.Left, left, pane(focusBin, fw, bh).Render(a.bin.view(fw-4, bh-2)))
		}
		panes = append(panes, left)
	}
	if rw > 0 {
		panes = append(panes, pane(focusResults, rw, h).Render(a.resultsView(rw-4)))
	}

	bindings := formHelp(a.form.canComplete())
	switch {
	case a.dialog != nil:
		bindings = dialogHelp()
	case a.focus == focusBin:
		bindings = a.bin.ShortHelp()
	case a.focus == focusResults:
		bindings = a.tree.keys.ShortHelp()
	}
	out := lipgloss.JoinHorizontal(lipgloss.Top, panes...) + "\n" + a.help.ShortHelpView(bindings)

	// A dialog floats centred over the window.
	if a.dialog != nil {
		d := a.dialog.view(a.styles, a.width-2)
		x := max(0, (a.width-lipgloss.Width(d))/2)
		y := max(0, (h-lipgloss.Height(d))/2)
		out = lipgloss.NewCompositor(
			lipgloss.NewLayer(out),
			lipgloss.NewLayer(d).X(x).Y(y).Z(1),
		).Render()
	}

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
	case a.restored && a.cancelled:
		status = "cancelled, saved " + humanize.Time(a.scannedAt)
	case a.restored:
		status = "saved " + humanize.Time(a.scannedAt)
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
	if p.Errors > 0 {
		parts = append(parts, s.err.Render(fmt.Sprintf("%d unreadable (Full Disk Access?)", p.Errors)))
	}
	if a.stale() {
		parts = append(parts, s.err.Render("press Search to rescan"))
	}
	if a.err != nil {
		parts = append(parts, s.err.Render(a.err.Error()))
	}
	header := strings.Join(parts, s.subtle.Render(" · "))
	header = lipgloss.NewStyle().MaxWidth(w).Render(header)
	return header + "\n\n" + a.tree.View()
}
