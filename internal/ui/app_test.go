package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestNewValidatesWhenSkippingForm(t *testing.T) {
	if _, err := New(Config{Roots: "/nope/missing", Min: "1MB", Type: "all", SkipForm: true}); err == nil {
		t.Error("expected error for missing root")
	}
	if _, err := New(Config{Roots: "~", Min: "1MB", Type: "bogus"}); err == nil {
		t.Error("expected error for bad type")
	}
}

func TestAppScanToTree(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "big"), make([]byte, 50_000), 0o644)
	a, err := New(Config{Roots: dir, Min: "1KB", Type: "all", SkipForm: true})
	if err != nil {
		t.Fatal(err)
	}
	if a.state != stateScanning {
		t.Fatalf("state = %v", a.state)
	}
	deadline := time.After(5 * time.Second)
	for a.state == stateScanning {
		select {
		case <-deadline:
			t.Fatal("scan did not finish")
		default:
		}
		var msg tea.Msg = a.scan.wait()()
		a.Update(msg)
	}
	if a.state != stateTree {
		t.Fatalf("state = %v, err = %v", a.state, a.err)
	}
	if len(a.tree.rows) < 2 {
		t.Errorf("rows = %d", len(a.tree.rows))
	}
	a.Update(press("n"))
	a.Update(newSearchMsg{})
	if a.state != stateForm {
		t.Errorf("state after new search = %v", a.state)
	}
}
