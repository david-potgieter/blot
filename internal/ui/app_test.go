package ui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"

	"github.com/david-potgieter/heft/internal/tree"
)

func TestNewValidatesWhenSkippingForm(t *testing.T) {
	if _, err := New(Config{Roots: "/nope/missing", Min: "1MB", Type: "all", SkipForm: true}); err == nil {
		t.Error("expected error for missing root")
	}
	if _, err := New(Config{Roots: "~", Min: "1MB", Type: "bogus"}); err == nil {
		t.Error("expected error for bad type")
	}
}

// fixtureDir holds big (3MB), mid/f (1.5MB) and small (10KB).
func fixtureDir(t *testing.T) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	for name, n := range map[string]int{"big": 3_000_000, "mid/f": 1_500_000, "small": 10_000} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func newApp(t *testing.T, cfg Config) *App {
	t.Helper()
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return a
}

// finish waits for the current job and delivers its done message.
func finish(t *testing.T, a *App) {
	t.Helper()
	select {
	case <-a.job.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not finish")
	}
	a.Update(jobDoneMsg{a.jobID})
}

func names(a *App) []string {
	var out []string
	for _, r := range a.tree.rows {
		out = append(out, r.Node.Name)
	}
	return out
}

func TestSubmitScansAndFocusesResults(t *testing.T) {
	dir := fixtureDir(t)
	a := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all"})
	if a.focus != focusForm || a.job != nil {
		t.Fatal("should start on the form with no scan")
	}
	if !strings.Contains(a.View().Content, "Set a search") {
		t.Error("missing pre-scan hint")
	}
	a.Update(submitMsg{})
	if !a.scanning || a.focus != focusResults || !a.form.scanning {
		t.Fatalf("scanning=%v focus=%v", a.scanning, a.focus)
	}
	finish(t, a)
	if a.scanning || a.form.scanning {
		t.Error("still scanning after done")
	}
	if got := strings.Join(names(a), ","); got != dir+",big,mid" {
		t.Errorf("rows = %s", got)
	}
}

func TestFilterEditsApplyWithoutRescan(t *testing.T) {
	dir := fixtureDir(t)
	a := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all", SkipForm: true})
	finish(t, a)
	id := a.jobID

	a.Update(keyTab)  // results → form (roots field)
	a.Update(keyDown) // min field
	for range len("1MB") {
		a.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	for _, r := range "2MB" {
		a.Update(press(string(r)))
	}
	if a.jobID != id {
		t.Fatal("filter edit started a new scan")
	}
	if got := strings.Join(names(a), ","); got != dir+",big" {
		t.Errorf("rows after min 2MB = %s", got)
	}

	a.Update(keyDown)  // type field
	a.Update(keyRight) // dirs only
	if a.filter.Type != tree.Dirs {
		t.Fatalf("type = %v", a.filter.Type)
	}
	if got := strings.Join(names(a), ","); got != dir {
		t.Errorf("rows with dirs ≥ 2MB = %s", got)
	}
}

func TestMinBelowFloorAsksForRescan(t *testing.T) {
	dir := fixtureDir(t)
	a := newApp(t, Config{Roots: dir, Min: "5MB", Type: "all", SkipForm: true})
	finish(t, a)
	if a.floor != maxFloor {
		t.Fatalf("floor = %d", a.floor)
	}
	a.form.min.SetValue("500KB")
	if !a.stale() || !strings.Contains(a.View().Content, "press Search") {
		t.Error("expected rescan hint for min below floor")
	}
	a.form.min.SetValue("2MB")
	a.form.roots.SetValue(dir + ", " + t.TempDir())
	if !a.stale() {
		t.Error("expected stale after changing folders")
	}
}

func TestFocusSwitching(t *testing.T) {
	a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all"})
	a.Update(keyEsc)
	if a.focus != focusForm {
		t.Error("esc with no results should stay on form")
	}
	a.Update(submitMsg{})
	finish(t, a)
	a.Update(keyTab)
	if a.focus != focusForm || !a.form.focused {
		t.Fatal("tab should focus form")
	}
	a.Update(keyEsc)
	if a.focus != focusResults || a.form.focused {
		t.Fatal("esc should focus results")
	}
	_, cmd := a.Update(press("q"))
	if cmd == nil {
		t.Fatal("q in results should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q should return tea.Quit")
	}
}

func TestSubmitWhileScanningCancels(t *testing.T) {
	a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all", SkipForm: true})
	a.Update(submitMsg{}) // the button shows Cancel while scanning
	if !a.cancelled {
		t.Fatal("submit while scanning should cancel")
	}
	finish(t, a)
	if a.scanning || a.form.scanning {
		t.Error("still scanning after cancel")
	}
}

func TestTickStreamsSnapshot(t *testing.T) {
	a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all", SkipForm: true})
	<-a.job.Done()
	a.Update(tickMsg{a.jobID}) // tick before the done message still shows results
	if len(a.tree.rows) < 2 {
		t.Errorf("rows after tick = %d", len(a.tree.rows))
	}
	a.Update(tickMsg{a.jobID - 1}) // stale tick ignored
}

func TestTypingInFormDoesNotTriggerResultKeys(t *testing.T) {
	a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all", SkipForm: true})
	finish(t, a)
	id := a.jobID
	a.Update(keyTab) // focus form (Folders)
	for _, k := range []string{"q", "r", "s"} {
		_, cmd := a.Update(press(k))
		if cmd != nil {
			if _, ok := cmd().(tea.QuitMsg); ok {
				t.Fatalf("%q in form quit", k)
			}
		}
	}
	if a.jobID != id || a.tree.tree.Sort() != tree.BySize {
		t.Error("form typing triggered rescan or sort")
	}
	if !strings.HasSuffix(a.form.roots.Value(), "qrs") {
		t.Errorf("roots = %q", a.form.roots.Value())
	}
}

func TestSubmitWhileScanningRestartsWhenFormChanged(t *testing.T) {
	dir := fixtureDir(t)
	a := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all", SkipForm: true})
	old := a.jobID
	a.form.roots.SetValue(t.TempDir())
	if a.scanning && !a.stale() {
		t.Fatal("expected stale")
	}
	a.Update(submitMsg{})
	if a.jobID != old+1 {
		t.Fatalf("expected a new scan, id %d → %d", old, a.jobID)
	}
	a.Update(jobDoneMsg{old}) // the old job's done message is ignored
	finish(t, a)
	if a.err != nil || a.scanning {
		t.Errorf("err=%v scanning=%v", a.err, a.scanning)
	}
}

func TestTickKeepsExpansionAndCursor(t *testing.T) {
	a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all", SkipForm: true})
	finish(t, a)
	a.Update(press("down"))  // big (largest child)
	a.Update(press("down"))  // mid
	a.Update(press("right")) // expand mid
	a.Update(press("down"))  // mid/f
	want := a.tree.current().Path
	a.scanning = true // pretend a tick arrives mid-scan
	a.Update(tickMsg{a.jobID})
	if got := a.tree.current().Path; got != want {
		t.Errorf("cursor moved to %s, want %s", got, want)
	}
}

func TestRaisingMinMovesCursorToAncestor(t *testing.T) {
	dir := fixtureDir(t)
	a := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all", SkipForm: true})
	finish(t, a)
	a.Update(press("down"))
	a.Update(press("down"))
	a.Update(press("right"))
	a.Update(press("down")) // mid/f (1.5MB)
	a.form.min.SetValue("2MB")
	a.rebuild(false)
	if got := a.tree.current().Path; got != dir {
		t.Errorf("cursor on %s, want root %s", got, dir)
	}
}

// assertOverlay checks the dialog floats over the UI rather than replacing it.
func assertOverlay(t *testing.T, a *App, button string) {
	t.Helper()
	out := a.View().Content
	if !strings.Contains(out, button) {
		t.Errorf("dialog missing %q", button)
	}
	if !strings.Contains(out, "Search") || !strings.Contains(out, "Results") || !strings.Contains(out, "Bin") {
		t.Error("panes not visible behind the dialog")
	}
	if lipgloss.Height(out) != a.height || lipgloss.Width(out) != a.width {
		t.Errorf("view is %dx%d, window %dx%d", lipgloss.Width(out), lipgloss.Height(out), a.width, a.height)
	}
	lines := strings.Split(out, "\n")
	if strings.Contains(lines[0], "Full Disk") || strings.Contains(lines[0], "Rescan") {
		t.Error("dialog drawn at the top instead of centred")
	}
}

func TestRescanFromHere(t *testing.T) {
	dir := fixtureDir(t)
	a := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all", SkipForm: true})
	finish(t, a)
	a.Update(press("down"))
	a.Update(press("down"))
	a.Update(press("right"))
	a.Update(press("down")) // mid/f: a file, so "here" is mid
	id := a.jobID
	a.Update(press("r"))
	if a.dialog == nil || a.jobID != id {
		t.Fatal("r should open the dialog without scanning")
	}
	assertOverlay(t, a, "From here")
	a.Update(keyEnter)
	if a.dialog != nil || a.jobID != id+1 {
		t.Fatal("enter should close the dialog and scan")
	}
	if want := filepath.Join(dir, "mid"); len(a.jobParams.Roots) != 1 || a.jobParams.Roots[0] != want {
		t.Errorf("roots = %v, want %s", a.jobParams.Roots, want)
	}
	finish(t, a)
}

func TestRescanAllAndCancel(t *testing.T) {
	dir := fixtureDir(t)
	a := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all", SkipForm: true})
	finish(t, a)
	id := a.jobID
	a.Update(press("r"))
	a.Update(keyEsc)
	if a.dialog != nil || a.jobID != id || a.focus != focusResults {
		t.Fatal("esc should just close the dialog")
	}
	a.Update(press("r"))
	a.Update(keyRight) // All
	a.Update(keyEnter)
	if a.jobID != id+1 || a.jobParams.Roots[0] != dir || a.form.roots.Value() != dir {
		t.Errorf("all: id=%d roots=%v form=%q", a.jobID, a.jobParams.Roots, a.form.roots.Value())
	}
	finish(t, a)
}

func TestTabCyclesPanes(t *testing.T) {
	a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all", Bin: t.TempDir()})
	a.Update(keyTab)
	if a.focus != focusBin || !a.bin.focused || a.form.focused {
		t.Fatalf("form → %d, want bin", a.focus)
	}
	a.Update(keyTab)
	if a.focus != focusForm {
		t.Fatalf("bin → %d, want form (no results yet)", a.focus)
	}
	a.Update(submitMsg{})
	finish(t, a)
	for _, want := range []focus{focusForm, focusBin, focusResults} {
		a.Update(keyTab)
		if a.focus != want {
			t.Fatalf("focus = %d, want %d", a.focus, want)
		}
	}
	a.Update(keyShiftTab)
	if a.focus != focusBin {
		t.Fatalf("shift+tab → %d, want bin", a.focus)
	}
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 12}) // too short for the Bin
	if a.focus != focusForm {
		t.Errorf("bin hidden but still focused")
	}
}

func TestBinPane(t *testing.T) {
	bin := fixtureDir(t)
	a := newApp(t, Config{Roots: bin, Min: "1MB", Type: "all", Bin: bin})
	if !strings.Contains(a.View().Content, "measuring") {
		t.Error("expected measuring before the first result")
	}
	msg := a.refreshBin()()
	a.Update(msg)
	if a.bin.files != 3 || len(a.bin.items) != 3 || a.bin.items[0].Name != "big" {
		t.Fatalf("files=%d items=%v", a.bin.files, a.bin.items)
	}
	if out := a.View().Content; !strings.Contains(out, "3 files") || !strings.Contains(out, "mid") {
		t.Errorf("bin pane missing totals or items:\n%s", out)
	}
	a.Update(binMsg{id: a.bin.id - 1}) // stale result ignored
	if a.bin.files != 3 {
		t.Error("stale bin result applied")
	}
	if a.bin.measuring {
		t.Fatal("still measuring after result")
	}
	a.Update(binPollMsg{binModTime(bin)}) // unchanged: no re-measure
	if a.bin.measuring {
		t.Error("re-measured an unchanged bin")
	}
	if err := os.WriteFile(filepath.Join(bin, "new"), make([]byte, 2_000_000), 0o644); err != nil {
		t.Fatal(err)
	}
	a.Update(binPollMsg{binModTime(bin)})
	if !a.bin.measuring || !strings.Contains(a.View().Content, "measuring") {
		t.Fatal("a changed bin should be re-measured")
	}
	a.Update(a.refreshBin()())
	if a.bin.files != 4 || a.bin.items[0].Name != "big" || a.bin.items[1].Name != "new" {
		t.Fatalf("after trashing: files=%d", a.bin.files)
	}

	a.Update(keyTab) // focus bin
	a.Update(press("down"))
	if a.bin.cursor != 1 {
		t.Errorf("cursor = %d", a.bin.cursor)
	}
	a.Update(binMsg{id: a.bin.id, err: errors.New("boom")})
	if out := a.View().Content; !strings.Contains(out, "can't read") || strings.Contains(out, "Full Disk") {
		t.Error("expected plain unreadable hint")
	}
	a.Update(keyEnter)
	if a.dialog != nil {
		t.Error("enter offered access for a non-permission error")
	}
}

func TestTrashedResultsDropOut(t *testing.T) {
	dir := fixtureDir(t)
	bin := t.TempDir()
	a := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all", SkipForm: true, Bin: bin})
	finish(t, a)
	total := a.tree.rows[0].Node.Size
	files := a.progress.Files
	if err := os.Rename(filepath.Join(dir, "mid"), filepath.Join(bin, "mid")); err != nil {
		t.Fatal(err)
	}
	_, cmd := a.Update(a.refreshBin()()) // the Bin changed
	if cmd == nil {
		t.Fatal("no check for gone results")
	}
	a.Update(cmd())
	if got := strings.Join(names(a), ","); got != dir+",big" {
		t.Errorf("rows = %s", got)
	}
	if r := a.tree.rows[0].Node; r.Size >= total-1_500_000 {
		t.Errorf("root size %d not reduced from %d", r.Size, total)
	}
	if a.progress.Files != files-1 || !strings.Contains(a.View().Content, humanize.Comma(files-1)+" files") {
		t.Errorf("files = %d, want %d", a.progress.Files, files-1)
	}
}

func TestQuitSavesAndPlainStartRestores(t *testing.T) {
	dir := fixtureDir(t)
	path := filepath.Join(t.TempDir(), "heft", "session.gob.gz")
	a := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all", SkipForm: true, Session: path, Bin: t.TempDir()})
	finish(t, a)
	a.Update(press("s"))     // by name: big, mid
	a.Update(press("down"))  // big
	a.Update(press("down"))  // mid
	a.Update(press("right")) // expand mid
	a.Update(press("down"))  // mid/f
	wantRows, wantCursor := strings.Join(names(a), ","), a.tree.current().Path
	if _, cmd := a.Update(press("q")); cmd == nil {
		t.Fatal("q should quit")
	}

	b := newApp(t, Config{Roots: "~", Min: "100MB", Type: "dirs", Session: path, Restore: true})
	if b.job == nil || !b.restored {
		t.Fatal("no results restored")
	}
	if got := strings.Join(names(b), ","); got != wantRows {
		t.Errorf("rows = %s, want %s", got, wantRows)
	}
	if got := b.tree.current().Path; got != wantCursor {
		t.Errorf("cursor = %s, want %s", got, wantCursor)
	}
	if b.tree.tree.Sort() != tree.ByName || b.focus != focusResults {
		t.Errorf("sort=%v focus=%d", b.tree.tree.Sort(), b.focus)
	}
	if b.form.roots.Value() != dir || b.form.min.Value() != "1MB" || b.form.typ != tree.All {
		t.Errorf("form = %+v", b.form.values())
	}
	if b.progress.Files != a.progress.Files || b.floor != a.floor || b.stale() {
		t.Errorf("files=%d floor=%d stale=%v", b.progress.Files, b.floor, b.stale())
	}
	if out := b.View().Content; !strings.Contains(out, "saved") {
		t.Error("header should say the results are saved")
	}

	// Filters still reach everything the scan kept.
	b.form.min.SetValue("2MB")
	b.rebuild(false)
	if got := strings.Join(names(b), ","); got != dir+",big" {
		t.Errorf("rows at 2MB = %s", got)
	}

	// Any paths or flags start fresh.
	if c := newApp(t, Config{Roots: dir, Min: "1MB", Type: "all", Session: path}); c.job != nil {
		t.Error("restored without Restore")
	}
}

func TestQuitMidScanSavesPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s")
	a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all", SkipForm: true, Session: path})
	a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	b := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Session: path, Restore: true})
	if b.job == nil || !b.cancelled || !strings.Contains(b.View().Content, "cancelled, saved") {
		t.Errorf("job=%v cancelled=%v", b.job != nil, b.cancelled)
	}
}

func TestBadSessionStartsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s")
	if err := os.WriteFile(path, []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Session: path, Restore: true})
	if a.job != nil || a.focus != focusForm || a.form.roots.Value() != "~" {
		t.Error("bad session should be ignored")
	}
	if err := writeSession(path, &session{Version: sessionVersion + 1, Roots: "/x"}); err != nil {
		t.Fatal(err)
	}
	if a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Session: path, Restore: true}); a.form.roots.Value() != "~" {
		t.Error("session from another version should be ignored")
	}
}

func TestRestoredScrollPosition(t *testing.T) {
	m := newTreeModel(newStyles(true))
	m.setTree(tree.New(testRoots(), tree.Filter{}))
	m.tree.Expand(m.rows[1].Node)
	m.refresh()
	m.follow(m.rows[len(m.rows)-1].Node.Path)
	m.wantOffset = 1
	m.refresh() // no size yet: kept for later
	if m.wantOffset != 1 {
		t.Fatal("offset applied before the size was known")
	}
	m.setSize(80, 3)
	if m.offset != 1 || m.wantOffset != -1 {
		t.Errorf("offset=%d want=%d", m.offset, m.wantOffset)
	}
}

func TestBinAccessDialog(t *testing.T) {
	a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all", Bin: t.TempDir()})
	a.Update(binMsg{id: a.bin.id, err: &fs.PathError{Op: "open", Path: "x", Err: syscall.EPERM}})
	if !strings.Contains(a.View().Content, "needs Full Disk Access") {
		t.Fatal("expected access hint")
	}
	a.Update(keyTab) // focus bin
	a.Update(keyEnter)
	if a.dialog == nil {
		t.Fatal("enter should open the access dialog")
	}
	assertOverlay(t, a, "Open Settings")
	a.Update(keyEsc)
	if a.dialog != nil || a.focus != focusBin {
		t.Fatal("esc should close the dialog")
	}
	a.Update(keyEnter)
	a.Update(keyRight) // Not now
	if _, cmd := a.Update(keyEnter); cmd != nil || a.dialog != nil {
		t.Error("Not now should just close")
	}
	a.Update(keyEnter)
	if _, cmd := a.Update(keyEnter); cmd == nil { // Open Settings (not run here)
		t.Error("Open Settings should return a command")
	}
}

func TestFormFitsPane(t *testing.T) {
	f := newTestForm()
	f.setWidth(formWidth - 4)
	f.roots.SetValue(strings.Repeat("x", 100))
	for _, line := range strings.Split(f.View(), "\n") {
		if w := lipgloss.Width(line); w > formWidth-4 && !strings.Contains(line, "not found") {
			t.Errorf("line %q is %d wide, pane fits %d", line, w, formWidth-4)
		}
	}
}

func TestViewFitsWindow(t *testing.T) {
	for _, size := range [][2]int{{120, 20}, {70, 20}, {120, 12}} {
		w, h := size[0], size[1]
		a := newApp(t, Config{Roots: fixtureDir(t), Min: "1MB", Type: "all", SkipForm: true})
		a.Update(tea.WindowSizeMsg{Width: w, Height: h})
		finish(t, a)
		for _, f := range []focus{focusResults, focusForm, focusBin} {
			a.setFocus(f)
			out := a.View().Content
			if got := lipgloss.Height(out); got != h {
				t.Errorf("w=%d h=%d focus=%d: height = %d", w, h, f, got)
			}
			if got := lipgloss.Width(out); got != w {
				t.Errorf("w=%d focus=%d: width = %d, want %d", w, f, got, w)
			}
		}
	}
}
