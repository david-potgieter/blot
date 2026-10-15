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

// startedApp returns an app scanning a temp dir holding one 50KB file.
func startedApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big"), make([]byte, 50_000), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{Roots: dir, Min: "1KB", Type: "all", SkipForm: true})
	if err != nil {
		t.Fatal(err)
	}
	if a.state != stateScanning {
		t.Fatalf("state = %v", a.state)
	}
	return a
}

// nextScanMsg waits for the scan's next message, failing instead of hanging.
func nextScanMsg(t *testing.T, a *App) tea.Msg {
	t.Helper()
	ch := make(chan tea.Msg, 1)
	go func() { ch <- a.scan.wait()() }()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not finish")
		return nil
	}
}

func finishScan(t *testing.T, a *App) {
	t.Helper()
	for a.state == stateScanning {
		a.Update(nextScanMsg(t, a))
	}
}

// run executes cmd and feeds its message back into the app.
func run(a *App, cmd tea.Cmd) {
	if cmd != nil {
		if msg := cmd(); msg != nil {
			a.Update(msg)
		}
	}
}

func TestAppScanToTree(t *testing.T) {
	a := startedApp(t)
	finishScan(t, a)
	if a.state != stateTree {
		t.Fatalf("state = %v, err = %v", a.state, a.err)
	}
	if len(a.tree.rows) < 2 {
		t.Errorf("rows = %d", len(a.tree.rows))
	}

	_, cmd := a.Update(press("n"))
	run(a, cmd)
	if a.state != stateForm {
		t.Errorf("state after n = %v", a.state)
	}
}

func TestAppRescan(t *testing.T) {
	a := startedApp(t)
	finishScan(t, a)
	id := a.scanID
	_, cmd := a.Update(press("r"))
	run(a, cmd)
	if a.state != stateScanning || a.scanID != id+1 {
		t.Fatalf("after r: state = %v, id = %d", a.state, a.scanID)
	}
	finishScan(t, a)
	if a.state != stateTree {
		t.Errorf("state after rescan = %v", a.state)
	}
}

func TestAppEscCancelsScanAndIgnoresStaleResult(t *testing.T) {
	a := startedApp(t)
	old := a.scan
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.state != stateForm {
		t.Fatalf("state after esc = %v", a.state)
	}
	// The cancelled scan's done message must not leave the form.
	a.Update(scanDoneMsg{id: old.id, res: nil, err: nil})
	if a.state != stateForm || a.err != nil {
		t.Errorf("stale result changed state = %v, err = %v", a.state, a.err)
	}
}
