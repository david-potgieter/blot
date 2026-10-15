package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

	a.Update(keyTab) // results → form (roots field)
	a.Update(keyTab) // min field
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

	a.Update(keyTab)   // type field
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
		for _, f := range []focus{focusResults, focusForm} {
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
