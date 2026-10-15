package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	f, _ = formKeys(f, keyDown, keyDown, keyDown)
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

func TestFormTypeSelector(t *testing.T) {
	f := newTestForm()
	f, _ = formKeys(f, keyDown, keyDown, keyRight)
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
}
