package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/david-potgieter/heft/internal/tree"
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
	f, _ = formKeys(f, keyTab, keyTab, keyTab)
	if f.focus != fieldButton {
		t.Fatalf("focus = %d, want button", f.focus)
	}
	f, _ = formKeys(f, keyTab)
	if f.focus != fieldRoots || !f.roots.Focused() {
		t.Fatalf("focus = %d, want wrap to roots", f.focus)
	}
	f, _ = formKeys(f, keyShiftTab)
	if f.focus != fieldButton {
		t.Fatalf("shift+tab focus = %d", f.focus)
	}
}

func TestFormTyping(t *testing.T) {
	f := newTestForm()
	f, _ = formKeys(f, keyTab, press("1"))
	if got := f.values().min; got != "100MB1" {
		t.Errorf("min = %q", got)
	}
	if got := f.values().roots; got != "~" {
		t.Errorf("roots changed: %q", got)
	}
}

func TestFormTypeSelector(t *testing.T) {
	f := newTestForm()
	f, _ = formKeys(f, keyTab, keyTab, keyRight)
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
		f, _ = formKeys(f, keyTab)
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
