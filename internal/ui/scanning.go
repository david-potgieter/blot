package ui

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/dustin/go-humanize"

	"github.com/david-potgieter/heft/internal/scan"
	"github.com/david-potgieter/heft/internal/size"
)

// Messages carry the scan id so results from cancelled scans are ignored.
type progressMsg struct {
	id int
	p  scan.Progress
}

type scanDoneMsg struct {
	id  int
	res *scan.Result
	err error
}

type scanModel struct {
	id       int
	params   Params
	spinner  spinner.Model
	progress scan.Progress
	cancel   context.CancelFunc
	ch       chan scan.Progress
	done     chan scanDoneMsg
	started  time.Time
	styles   styles
}

// newScanModel starts the scan in the background.
func newScanModel(id int, p Params, excludes []string, st styles) scanModel {
	ctx, cancel := context.WithCancel(context.Background())
	m := scanModel{
		id:      id,
		params:  p,
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(st.title)),
		cancel:  cancel,
		ch:      make(chan scan.Progress, 1),
		done:    make(chan scanDoneMsg, 1),
		started: time.Now(),
		styles:  st,
	}
	go func() {
		res, err := scan.Scan(ctx, p.Roots, scan.Options{FileFloor: p.Min, Exclude: excludes}, m.ch)
		m.done <- scanDoneMsg{id: id, res: res, err: err}
	}()
	return m
}

func (m scanModel) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, m.wait()) }

func (m scanModel) wait() tea.Cmd {
	return func() tea.Msg {
		select {
		case p := <-m.ch:
			return progressMsg{id: m.id, p: p}
		case d := <-m.done:
			return d
		}
	}
}

func (m scanModel) Update(msg tea.Msg) (scanModel, tea.Cmd) {
	switch msg := msg.(type) {
	case progressMsg:
		if msg.id != m.id {
			return m, nil
		}
		m.progress = msg.p
		return m, m.wait()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m scanModel) View(width int) string {
	s := m.styles
	p := m.progress
	cur := p.Current
	if r := []rune(cur); width > 5 && len(r) > width-4 {
		cur = "…" + string(r[len(r)-(width-5):])
	}
	return fmt.Sprintf("%s %s %s\n\n  %s files · %s folders · %s · %s errors · %s\n\n  %s\n\n  %s",
		m.spinner.View(), s.title.Render("Scanning"), s.subtle.Render(m.params.summary()),
		humanize.Comma(p.Files), humanize.Comma(p.Dirs), size.Format(p.Bytes), humanize.Comma(p.Errors),
		time.Since(m.started).Round(time.Second),
		s.subtle.Render(cur),
		s.subtle.Render("esc cancel · ctrl+c quit"),
	)
}
