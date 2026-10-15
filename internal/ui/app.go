// Package ui is heft's Bubble Tea interface.
package ui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/david-potgieter/heft/internal/tree"
)

type state int

const (
	stateForm state = iota
	stateScanning
	stateTree
)

// Config holds initial form values (from CLI flags).
type Config struct {
	Roots    string // comma-separated
	Min      string // e.g. "500MB"
	Type     string // all | dirs | files
	SkipForm bool
	Excludes []string
}

// App is the root model: form → scanning → tree.
type App struct {
	cfg           Config
	state         state
	values        *formValues
	form          *huh.Form
	scan          scanModel
	scanID        int
	tree          treeModel
	params        Params
	styles        styles
	width, height int
	err           error
	initCmd       tea.Cmd // from New's showForm/startScan, returned once by Init
	isDark        bool
}

// New builds the app. With SkipForm it validates cfg and starts scanning.
func New(cfg Config) (*App, error) {
	typ, err := tree.ParseType(cfg.Type)
	if err != nil {
		return nil, err
	}
	a := &App{cfg: cfg, values: &formValues{roots: cfg.Roots, min: cfg.Min, typ: typ}, styles: newStyles(true), isDark: true}
	if cfg.SkipForm {
		p, err := a.values.params()
		if err != nil {
			return nil, err
		}
		a.initCmd = a.startScan(p)
		return a, nil
	}
	a.initCmd = a.showForm()
	return a, nil
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, a.initCmd)
}

func (a *App) showForm() tea.Cmd {
	// Theme reads a.isDark at render time, so forms built before or after
	// background detection both follow it.
	a.form = newForm(a.values).WithTheme(huh.ThemeFunc(func(bool) *huh.Styles {
		return huh.ThemeCharm(a.isDark)
	}))
	a.state = stateForm
	return a.form.Init()
}

func (a *App) startScan(p Params) tea.Cmd {
	a.params = p
	a.err = nil
	a.scanID++
	a.scan = newScanModel(a.scanID, p, a.cfg.Excludes, a.styles)
	a.state = stateScanning
	return a.scan.Init()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		a.isDark = msg.IsDark()
		a.styles = newStyles(a.isDark)
		a.tree.styles = a.styles
		a.scan.styles = a.styles
		a.scan.spinner.Style = a.styles.title
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if a.state == stateScanning {
				a.scan.cancel()
			}
			return a, tea.Quit
		}
	}

	switch a.state {
	case stateForm:
		m, cmd := a.form.Update(msg)
		a.form = m.(*huh.Form)
		switch a.form.State {
		case huh.StateCompleted:
			p, err := a.values.params()
			if err != nil {
				a.err = err
				return a, a.showForm()
			}
			return a, a.startScan(p)
		case huh.StateAborted:
			return a, tea.Quit
		}
		return a, cmd

	case stateScanning:
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			if msg.String() == "esc" {
				a.scan.cancel()
				return a, a.showForm()
			}
		case scanDoneMsg:
			if msg.id != a.scanID {
				return a, nil
			}
			a.scan.cancel() // release the context
			if msg.err != nil {
				if !errors.Is(msg.err, context.Canceled) {
					a.err = msg.err
				}
				return a, a.showForm()
			}
			a.tree = newTreeModel(msg.res, a.params, a.styles, a.width, a.height)
			a.state = stateTree
			return a, nil
		}
		var cmd tea.Cmd
		a.scan, cmd = a.scan.Update(msg)
		return a, cmd

	case stateTree:
		switch msg.(type) {
		case rescanMsg:
			return a, a.startScan(a.params)
		case newSearchMsg:
			return a, a.showForm()
		}
		var cmd tea.Cmd
		a.tree, cmd = a.tree.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (a *App) View() tea.View {
	var s string
	switch a.state {
	case stateForm:
		s = a.styles.title.Render("heft") + "  " + a.styles.subtle.Render("find big files and folders") + "\n\n"
		if a.err != nil {
			s += a.styles.err.Render(a.err.Error()) + "\n\n"
		}
		s += a.form.View()
	case stateScanning:
		s = a.scan.View(a.width)
	case stateTree:
		s = a.tree.View()
	}
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}
