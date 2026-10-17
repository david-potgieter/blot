package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/david-potgieter/blot/internal/queries"
	"github.com/david-potgieter/blot/internal/tree"
)

func formKeys(f formModel, keys ...tea.KeyPressMsg) (formModel, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		f, cmd = f.Update(k)
	}
	return f, cmd
}

var (
	keyTab      = tea.KeyPressMsg{Code: tea.KeyTab}
	keyShiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	keyDown     = tea.KeyPressMsg{Code: tea.KeyDown}
	keyUp       = tea.KeyPressMsg{Code: tea.KeyUp}
	keyEnter    = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyRight    = tea.KeyPressMsg{Code: tea.KeyRight}
	keyLeft     = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyEsc      = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func newTestForm() formModel {
	f := newFormModel(formValues{roots: "~", min: "100MB", typ: tree.All}, newStyles(true))
	f.setFocused(true)
	return f
}

func TestFormFieldNavigationWraps(t *testing.T) {
	f := newTestForm()
	f, _ = formKeys(f, keyDown, keyDown, keyDown, keyDown)
	if f.focus != fieldButton {
		t.Fatalf("focus = %d, want button", f.focus)
	}
	f, _ = formKeys(f, keyDown)
	if f.focus != fieldRoots || !f.roots.Focused() {
		t.Fatalf("focus = %d, want wrap to roots", f.focus)
	}
	f, _ = formKeys(f, keyUp)
	if f.focus != fieldButton {
		t.Fatalf("shift+tab focus = %d", f.focus)
	}
}

func TestFormTyping(t *testing.T) {
	f := newTestForm()
	f, _ = formKeys(f, keyDown, press("1"))
	if got := f.values().min; got != "100MB1" {
		t.Errorf("min = %q", got)
	}
	if got := f.values().roots; got != "~" {
		t.Errorf("roots changed: %q", got)
	}
}

func TestFormNameField(t *testing.T) {
	f := newTestForm()
	out := f.View()
	if i, j, k := strings.Index(out, "Min size"), strings.Index(out, "Name"), strings.Index(out, "Show"); !(i < j && j < k) {
		t.Errorf("Name not between Min size and Show:\n%s", out)
	}
	if got := f.name.Placeholder; got != "node_modules, .next" {
		t.Errorf("placeholder = %q", got)
	}
	f, _ = formKeys(f, keyDown, keyDown)
	if !f.name.Focused() {
		t.Fatalf("focus = %d, want name after min", f.focus)
	}
	f, _ = formKeys(f, press("n"), press("m"))
	if got := f.values(); got.name != "nm" || got.min != "100MB" {
		t.Errorf("values = %+v", got)
	}
	f, _ = formKeys(f, keyUp)
	if !f.min.Focused() {
		t.Errorf("up from name: focus = %d, want min", f.focus)
	}
}

func TestFormTypeSelector(t *testing.T) {
	f := newTestForm()
	f, _ = formKeys(f, keyDown, keyDown, keyDown, keyRight)
	if f.typ != tree.Dirs {
		t.Fatalf("typ = %v", f.typ)
	}
	f, _ = formKeys(f, keyLeft, keyLeft)
	if f.typ != tree.Files {
		t.Fatalf("typ = %v, want wrap to files", f.typ)
	}
}

func TestFormEnterSubmitsFromAnyField(t *testing.T) {
	f := newTestForm()
	for range numFields {
		_, cmd := formKeys(f, keyEnter)
		if cmd == nil {
			t.Fatalf("field %d: no cmd", f.focus)
		}
		if _, ok := cmd().(submitMsg); !ok {
			t.Fatalf("field %d: enter did not submit", f.focus)
		}
		f, _ = formKeys(f, keyDown)
	}
}

// typeRoots replaces the Folders value by typing it, as a user would.
func typeRoots(f formModel, s string) formModel {
	f.roots.SetValue("")
	for _, r := range s {
		f, _ = f.Update(press(string(r)))
	}
	return f
}

func TestFolderCompletion(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"Work", "Workspace", "Music", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFileT(t, filepath.Join(dir, "Wordfile"))
	if err := os.Symlink(filepath.Join(dir, "Music"), filepath.Join(dir, "Wlink")); err != nil {
		t.Fatal(err)
	}
	f := newTestForm()

	for typed, want := range map[string][]string{
		dir + "/W":               {dir + "/Wlink/", dir + "/Work/", dir + "/Workspace/"}, // folders and links to them, not files
		dir + "/w":               {dir + "/Wlink/", dir + "/Work/", dir + "/Workspace/"}, // case-insensitive
		dir + "/Work":            {dir + "/Workspace/"},                                  // the exact name isn't offered
		dir + "/":                nil,                                                    // nothing typed of the name yet
		dir + "/.h":              {dir + "/.hidden/"},
		"~, " + dir + "/Mu":      {"~, " + dir + "/Music/"}, // after a comma, keeping what's before it
		dir + "/M, " + dir + "/": nil,
	} {
		f = typeRoots(f, typed)
		if got := f.roots.MatchedSuggestions(); !slices.Equal(got, want) {
			t.Errorf("%q: suggestions = %q, want %q", typed, got, want)
		}
	}
	f = typeRoots(f, dir+"/")
	if slices.ContainsFunc(f.roots.AvailableSuggestions(), func(s string) bool { return strings.Contains(s, ".hidden") }) {
		t.Error("hidden folder offered without a dot")
	}

	f = typeRoots(f, dir+"/mu")
	if !f.canComplete() {
		t.Fatal("expected a completion")
	}
	f, _ = f.Update(keyTab)
	if got := f.roots.Value(); got != dir+"/Music/" { // the folder's own case, not what was typed
		t.Errorf("after tab = %q", got)
	}
	if f.roots.Position() != len(f.roots.Value()) {
		t.Error("cursor not at the end after completing")
	}
	if f.canComplete() {
		t.Error("nothing typed of the next name, so tab should leave the field")
	}
}

func TestFolderCompletionWithCommas(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Photos, 2024"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := typeRoots(newTestForm(), dir+"/Pho")
	want := []string{dir + `/Photos\, 2024/`}
	if got := f.roots.MatchedSuggestions(); !slices.Equal(got, want) {
		t.Errorf("suggestions = %q, want %q", got, want)
	}
	// Typing the escaped comma keeps completing within that one path.
	f = typeRoots(f, dir+`/Photos\, 2`)
	if got := f.roots.MatchedSuggestions(); !slices.Equal(got, want) {
		t.Errorf("after escaped comma: suggestions = %q, want %q", got, want)
	}
}

func TestFolderCompletionSeesNewFolders(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Work"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := typeRoots(newTestForm(), dir+"/W")
	if err := os.Mkdir(filepath.Join(dir, "Wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Keep typing in the same folder (retyping the whole path would list
	// other folders on the way and evict the cache anyway).
	f, _ = f.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	f, _ = f.Update(press("W"))
	want := []string{dir + "/Wiki/", dir + "/Work/"}
	if got := f.roots.MatchedSuggestions(); !slices.Equal(got, want) {
		t.Errorf("suggestions = %q, want %q (folder created after the first listing)", got, want)
	}
}

func TestFolderCompletionIsCapped(t *testing.T) {
	dir := t.TempDir()
	for i := range maxSuggestions + 10 {
		if err := os.Mkdir(filepath.Join(dir, fmt.Sprintf("d%03d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := typeRoots(newTestForm(), dir+"/d")
	got := f.roots.MatchedSuggestions()
	if len(got) != maxSuggestions || got[0] != dir+"/d000/" {
		t.Errorf("got %d suggestions starting %q", len(got), got[0])
	}
}

func TestTabCompletesThenMovesOn(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Music"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Bin: t.TempDir()})
	a.form = typeRoots(a.form, dir+"/Mu")
	a.Update(keyTab)
	if a.focus != focusForm || a.form.roots.Value() != dir+"/Music/" {
		t.Fatalf("tab should complete: focus=%d value=%q", a.focus, a.form.roots.Value())
	}
	a.Update(keyTab)
	if a.focus != focusBin {
		t.Errorf("second tab should move on, focus=%d", a.focus)
	}
}

func writeFileT(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// savedQueries writes a queries file holding labels and returns its path.
func savedQueries(t *testing.T, labels ...string) string {
	t.Helper()
	s := queries.Store{Path: filepath.Join(t.TempDir(), "queries.json")}
	for _, l := range labels {
		if err := s.Save(l, queries.Query{Roots: "~", Min: "1MB", Show: "all"}); err != nil {
			t.Fatal(err)
		}
	}
	return s.Path
}

func TestSavedListShownBelowSearch(t *testing.T) {
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Bin: t.TempDir(), Queries: savedQueries(t, "photos", "builds")})
	out := a.View().Content
	btn, saved := strings.Index(out, "[ Search ]"), strings.Index(out, "Saved")
	b, p := strings.Index(out, "builds"), strings.Index(out, "photos")
	if !(btn >= 0 && btn < saved && saved < b && b < p) {
		t.Errorf("want Saved, builds, photos below Search:\n%s", out)
	}
}

func TestSavedListNavigation(t *testing.T) {
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Bin: t.TempDir(), Queries: savedQueries(t, "a", "b")})
	selected := func() string { return a.form.savedSelection() }
	for range 4 {
		a.Update(keyDown) // roots → button
	}
	if a.form.focus != fieldButton || selected() != "" {
		t.Fatalf("focus = %d, want button", a.form.focus)
	}
	a.Update(keyDown)
	if selected() != "a" {
		t.Fatalf("down from button: selected %q, want a", selected())
	}
	a.Update(keyDown)
	if selected() != "b" {
		t.Fatalf("down in list: selected %q, want b", selected())
	}
	a.Update(keyUp)
	a.Update(keyUp)
	if a.form.focus != fieldButton || selected() != "" {
		t.Fatalf("up from first row: focus = %d, want button", a.form.focus)
	}
	a.Update(keyDown)
	a.Update(keyTab)
	if a.focus != focusBin {
		t.Errorf("tab from saved list: focus = %d, want bin", a.focus)
	}
}

func TestSavedListReloadsAfterSaveAndDelete(t *testing.T) {
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Bin: t.TempDir(), Queries: savedQueries(t, "a")})
	if err := a.form.saveQuery("b"); err != nil {
		t.Fatal(err)
	}
	if out := a.form.View(); !strings.Contains(out, "  b") {
		t.Errorf("saved query not listed:\n%s", out)
	}
	if err := a.form.deleteSaved("a"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.form.saved, []string{"b"}) {
		t.Errorf("after delete: saved = %q, want [b]", a.form.saved)
	}
}

var keyCtrlS = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}

func TestCtrlSPrefillsName(t *testing.T) {
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Name: "node_modules", Bin: t.TempDir(), Queries: savedQueries(t)})
	a.Update(keyCtrlS)
	if a.dialog == nil || a.dialog.input == nil || a.dialog.input.Value() != "node_modules" {
		t.Fatal("ctrl+s should open the save prompt prefilled with Name")
	}
	assertOverlay(t, a, "node_modules")
}

func TestCtrlSPrefillsFirstFolderWhenNameBlank(t *testing.T) {
	home, _ := os.UserHomeDir()
	a := newApp(t, Config{Roots: filepath.Join(home, "Work") + ", /tmp", Min: "1MB", Type: "all", Bin: t.TempDir(), Queries: savedQueries(t)})
	a.Update(keyCtrlS)
	if a.dialog == nil || a.dialog.input.Value() != "~/Work" {
		t.Fatalf("want prompt prefilled with ~/Work, got %+v", a.dialog)
	}
}

// typeLabel replaces the save prompt's text with s.
func typeLabel(a *App, s string) {
	a.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	for _, r := range s {
		a.Update(press(string(r)))
	}
}

func TestCtrlSSavesQueryAndListsIt(t *testing.T) {
	path := savedQueries(t, "a")
	a := newApp(t, Config{Roots: "~", Min: "2MB", Type: "dirs", Name: "*.log", Bin: t.TempDir(), Queries: path})
	a.Update(keyCtrlS)
	typeLabel(a, "logs")
	a.Update(keyEnter)
	if a.dialog != nil {
		t.Fatal("enter should close the prompt")
	}
	got, err := queries.Store{Path: path}.Load("logs")
	if want := (queries.Query{Label: "logs", Roots: "~", Min: "2MB", Show: "dirs", Name: "*.log"}); err != nil || got != want {
		t.Errorf("saved %+v (%v), want %+v", got, err, want)
	}
	if !strings.Contains(a.View().Content, "logs") || !slices.Equal(a.form.saved, []string{"a", "logs"}) {
		t.Errorf("list not reloaded: %q", a.form.saved)
	}
}

func TestCtrlSEscCancels(t *testing.T) {
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Bin: t.TempDir(), Queries: savedQueries(t, "a")})
	a.Update(keyCtrlS)
	typeLabel(a, "q") // typed, not a quit
	a.Update(keyEsc)
	if a.dialog != nil || a.focus != focusForm || !slices.Equal(a.form.saved, []string{"a"}) {
		t.Errorf("esc: dialog=%v focus=%d saved=%q", a.dialog, a.focus, a.form.saved)
	}
}

func TestCtrlSAsksBeforeOverwriting(t *testing.T) {
	path := savedQueries(t, "a") // saved with Min 1MB
	a := newApp(t, Config{Roots: "~", Min: "5MB", Type: "all", Bin: t.TempDir(), Queries: path})
	savedMin := func() string {
		q, _ := queries.Store{Path: path}.Load("a")
		return q.Min
	}
	a.Update(keyCtrlS)
	typeLabel(a, "a")
	a.Update(keyEnter)
	if a.dialog == nil || !strings.Contains(a.View().Content, "Overwrite 'a'?") || savedMin() != "1MB" {
		t.Fatal("saving over a label should ask first")
	}
	a.Update(keyRight) // No
	a.Update(keyEnter)
	if a.dialog != nil || savedMin() != "1MB" {
		t.Fatal("no should cancel the save")
	}
	a.Update(keyCtrlS)
	typeLabel(a, "a")
	a.Update(keyEnter)
	a.Update(keyEnter) // Yes
	if a.dialog != nil || savedMin() != "5MB" {
		t.Errorf("yes should replace the query: min = %s", savedMin())
	}
}

// pressEnterOnSaved moves from Folders to the first Saved row, presses enter
// and delivers what that sends to the app.
func pressEnterOnSaved(a *App) {
	for range 5 {
		a.Update(keyDown) // roots → first saved row
	}
	_, cmd := a.Update(keyEnter)
	a.Update(cmd())
}

func TestEnterOnSavedRowRecallsAndScans(t *testing.T) {
	dir := fixtureDir(t)
	s := queries.Store{Path: filepath.Join(t.TempDir(), "queries.json")}
	if err := s.Save("mids", queries.Query{Roots: dir, Min: "1MB", Show: "dirs", Name: "mid"}); err != nil {
		t.Fatal(err)
	}
	a := newApp(t, Config{Roots: "~", Min: "500MB", Type: "all", Bin: t.TempDir(), Queries: s.Path})
	pressEnterOnSaved(a)
	if got := a.form.values(); got != (formValues{roots: dir, min: "1MB", name: "mid", typ: tree.Dirs}) {
		t.Errorf("form = %+v", got)
	}
	if !a.scanning || a.focus != focusResults {
		t.Fatalf("scanning=%v focus=%v, want a scan started", a.scanning, a.focus)
	}
	finish(t, a)
	if got := strings.Join(names(a), ","); got != dir+",mid" {
		t.Errorf("rows = %s", got)
	}
}

func TestTweakingRecalledQueryLeavesItSaved(t *testing.T) {
	dir := fixtureDir(t)
	s := queries.Store{Path: filepath.Join(t.TempDir(), "queries.json")}
	saved := queries.Query{Label: "mids", Roots: dir, Min: "1MB", Show: "all", Name: "mid"}
	if err := s.Save(saved.Label, saved); err != nil {
		t.Fatal(err)
	}
	a := newApp(t, Config{Roots: "~", Min: "500MB", Type: "all", Bin: t.TempDir(), Queries: s.Path})
	pressEnterOnSaved(a)
	finish(t, a)
	a.Update(keyTab) // results → form, still on the Saved row
	a.Update(keyUp)  // button
	a.Update(keyUp)  // show
	a.Update(keyUp)  // name
	for range 3 {
		a.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	for _, r := range "big" {
		a.Update(press(string(r)))
	}
	a.Update(submitMsg{})
	finish(t, a)
	if got := strings.Join(names(a), ","); got != dir+",big" {
		t.Errorf("rows = %s, want the tweaked name searched", got)
	}
	if got, _ := s.Load("mids"); got != saved {
		t.Errorf("saved query = %+v, want unchanged %+v", got, saved)
	}
}

func TestSavedListHiddenWhenNoQueries(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "queries.json")
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Bin: t.TempDir(), Queries: missing})
	if out := a.View().Content; strings.Contains(out, "Saved") {
		t.Errorf("Saved list shown with no queries file:\n%s", out)
	}
}

func TestSavedListCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.json")
	if err := os.WriteFile(path, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newApp(t, Config{Roots: "~", Min: "1MB", Type: "all", Bin: t.TempDir(), Queries: path})
	if out := a.form.View(); !strings.Contains(out, "corrupt queries file") { // unwrapped by the pane
		t.Errorf("no error shown:\n%s", out)
	}
	a.Update(keyUp) // roots → wraps past the button
	if a.form.focus != fieldButton {
		t.Errorf("up from roots: focus = %d, want button (list is empty)", a.form.focus)
	}
	if data, _ := os.ReadFile(path); string(data) != "{oops" {
		t.Errorf("file rewritten: %q", data)
	}
}

func TestFormButtonLabelAndErrors(t *testing.T) {
	f := newTestForm()
	if out := f.View(); !strings.Contains(out, "Search") || strings.Contains(out, "Cancel") {
		t.Errorf("idle view:\n%s", out)
	}
	f.scanning = true
	if out := f.View(); !strings.Contains(out, "Cancel") {
		t.Errorf("scanning view:\n%s", out)
	}
	f.min.SetValue("lots")
	if out := f.View(); !strings.Contains(out, "try 500MB") {
		t.Errorf("missing size error:\n%s", out)
	}
	f.name.SetValue("[abc")
	if out := f.View(); !strings.Contains(out, "[abc: invalid pattern") {
		t.Errorf("missing name error:\n%s", out)
	}
}
