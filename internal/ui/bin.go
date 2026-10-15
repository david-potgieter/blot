package ui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"

	"github.com/david-potgieter/heft/internal/scan"
	"github.com/david-potgieter/heft/internal/size"
)

// binPoll is how often the Bin is checked for changes. Only its modification
// time is read, which works even without Full Disk Access.
const binPoll = 2 * time.Second

type (
	// binMsg carries a finished measurement of the Bin.
	binMsg struct {
		id  int
		mod time.Time // the Bin's modification time when measuring began
		res *scan.Result
		err error
	}
	// binPollMsg carries the Bin's current modification time.
	binPollMsg struct{ mod time.Time }
)

func binModTime(dir string) time.Time {
	if st, err := os.Stat(dir); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}

// measureBin sizes everything in dir (the user's Trash).
func measureBin(dir string, id int) tea.Cmd {
	return func() tea.Msg {
		mod := binModTime(dir)
		res, err := scan.Scan(context.Background(), []string{dir}, scan.Options{}, nil)
		return binMsg{id, mod, res, err}
	}
}

func pollBin(dir string) tea.Cmd {
	return tea.Tick(binPoll, func(time.Time) tea.Msg { return binPollMsg{binModTime(dir)} })
}

// binModel lists what's in the Bin, biggest first.
type binModel struct {
	id        int       // latest measurement asked for; older results are dropped
	measuring bool      // a measurement is running
	mod       time.Time // the Bin's modification time as last measured
	loaded    bool
	err       error
	files     int64
	bytes     int64
	items     []*scan.Node
	cursor    int
	focused   bool
	keys      binKeys
	styles    styles
}

type binKeys struct {
	Up, Down, Reveal, Refresh, Access, Pane, Quit key.Binding
}

func newBinModel(st styles) binModel {
	return binModel{styles: st, keys: binKeys{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Reveal:  key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "reveal")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Access:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "grant access")),
		Pane:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "pane")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}}
}

func (m binModel) ShortHelp() []key.Binding {
	k := m.keys
	if m.noAccess() {
		return []key.Binding{k.Access, k.Refresh, k.Pane, k.Quit}
	}
	return []key.Binding{k.Up, k.Down, k.Reveal, k.Refresh, k.Pane, k.Quit}
}

// noAccess reports that macOS refused to list the Bin (no Full Disk Access).
func (m binModel) noAccess() bool { return errors.Is(m.err, fs.ErrPermission) }

// set takes a measurement. A Bin that can't be read keeps no items.
func (m *binModel) set(msg binMsg) {
	m.loaded, m.measuring, m.mod, m.err = true, false, msg.mod, msg.err
	m.files, m.bytes, m.items = 0, 0, nil
	if msg.res != nil && msg.err == nil {
		m.files, m.bytes = msg.res.Files, msg.res.Bytes
		if len(msg.res.Roots) > 0 {
			for _, c := range msg.res.Roots[0].Children {
				if c.Name != ".DS_Store" {
					m.items = append(m.items, c)
				}
			}
		}
	}
	slices.SortFunc(m.items, func(a, b *scan.Node) int {
		return cmp.Or(cmp.Compare(b.Size, a.Size), cmp.Compare(a.Name, b.Name))
	})
	m.cursor = max(0, min(m.cursor, len(m.items)-1))
}

func (m binModel) Update(msg tea.KeyPressMsg) (binModel, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.cursor = max(0, m.cursor-1)
	case key.Matches(msg, m.keys.Down):
		m.cursor = max(0, min(m.cursor+1, len(m.items)-1))
	case key.Matches(msg, m.keys.Reveal):
		if m.cursor < len(m.items) {
			return m, reveal(m.items[m.cursor].Path)
		}
	}
	return m, nil
}

// view renders the pane body in a w×h box, scrolling to keep the cursor visible.
func (m binModel) view(w, h int) string {
	s := m.styles
	header := s.title.Render("Bin")
	switch {
	case m.measuring || !m.loaded:
		header += s.subtle.Render(" · measuring…")
	case m.noAccess():
		header += " " + s.err.Render("needs Full Disk Access")
	case m.err != nil:
		header += " " + s.err.Render("can't read")
	default:
		header += s.subtle.Render(" · " + size.Format(m.bytes) + " · " + humanize.Comma(m.files) + " files")
	}
	lines := []string{header, ""}
	if m.noAccess() {
		hint := "Tab here, then press enter."
		if m.focused {
			hint = "Press enter to grant it."
		}
		lines = append(lines, s.subtle.Render(hint))
	}
	if m.loaded && m.err == nil && len(m.items) == 0 {
		lines = append(lines, s.subtle.Render("Empty"))
	}

	rows := max(0, h-len(lines))
	offset := max(0, m.cursor-rows+1)
	for i := offset; i < len(m.items) && i < offset+rows; i++ {
		line := fmt.Sprintf("%9s  %s", size.Format(m.items[i].Size), m.items[i].Name)
		if m.focused && i == m.cursor {
			line = s.selected.Render(line)
		}
		lines = append(lines, line)
	}
	for i, l := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(w).Render(l)
	}
	return strings.Join(lines, "\n")
}
