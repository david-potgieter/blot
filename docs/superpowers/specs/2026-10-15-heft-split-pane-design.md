# heft — split-pane v2 design

Builds on `2026-10-14-heft-design.md`. Replaces the form → scanning → tree screen flow with one split screen.

## Layout

```
┌ Search ────────────┐┏ Results ── scanning 12,403 files · 4.1 GB ━━┓
│ Folders            │┃  4.3 GB ████ ~/Work                         ┃
│ ~/Work             │┃  2.1 GB ██░░   ▸ a/node_modules/            ┃
│ Min size           │┃  1.3 GB █░░░   ▸ b/                         ┃
│ 500MB              │┃                                             ┃
│ Show  ‹ all ›      │┃                                             ┃
│ [ Search ]         │┃                                             ┃
└────────────────────┘┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
 help line (depends on focused pane)
```

- Left pane fixed ~34 cols: custom form (bubbles `textinput` ×2, Show selector, button). huh removed.
- Right pane: remaining width. Before first scan: hint. Otherwise: header line (status + counters) and tree.
- Focused pane border uses accent color; other pane subtle.
- Width < 90: show only focused pane.

## Form

Fields in order: Folders (text), Min size (text), Show (‹ all/dirs/files ›, ←/→ or h/l... only ←/→), Button.
- `tab` / `down` next field, `shift+tab` / `up` previous (wraps).
- `enter` anywhere = press button.
- Button label: `[ Search ]` idle, `[ Cancel ]` while scanning.
- Invalid Folders/Min size shown as inline error under the field; Search refuses to start while invalid.
- Prefilled from CLI flags (defaults `~`, `100MB`, `all`). `--no-form` → start scan immediately with focus on results.

## Focus

- Start: form (or results with `--no-form`).
- Search pressed → scan starts, focus moves to results.
- Results: `tab` → form. Form: `esc` → results (if any results/scan exist).
- `ctrl+c` quits anywhere; `q` quits from results only (it's typeable in the form).

## Live behaviour

1. **Streaming.** While scanning, every 250ms the UI takes `Snapshot(floor)` and rebuilds the tree. Expanded dirs and cursor are tracked by path, so they survive rebuilds. Header: `scanning · 12,403 files · 4.1 GB · 3 errors`.
2. **Instant filters.** Editing Min size / Show re-filters the current snapshot/result immediately (scanning or done), as long as the parsed min ≥ scan floor. Invalid input → keep last valid filter.
3. **Stale search.** If Folders differ from the scanned roots, or min < floor, header shows `press Search to rescan`.
4. **Search** cancels any running scan, starts a new one. **Cancel** stops scan, keeps partial results (header `cancelled`).
5. **Floor** = min(form min at Search time, 1MB).

## Engine

### scan

```go
func Start(ctx context.Context, roots []string, opts Options) *Job  // runs in background
func (s *Job) Progress() Progress
func (s *Job) Snapshot(min int64) []*Node   // deep copy of nodes with Size >= min (roots always)
func (s *Job) Done() <-chan struct{}
func (s *Job) Result() (*Result, error)     // valid after Done
func Scan(ctx, roots, opts, progress) (*Result, error) // kept: wraps Start, for tests/headless
```

- Live nodes are internal (`live` struct, all fields under the job mutex). File and dir usage is added to the node and every ancestor under the lock when found, so snapshots always satisfy parent ≥ sum of children. `sumDirs` removed. (One lock per file; benchmark on big trees later.)
- The UI snapshots at the current filter min (≥ floor), re-snapshotting when min changes, so per-tick cost tracks what is shown.
- `Snapshot` holds the scanner mutex while copying; copies only nodes with size ≥ min, so cost ∝ visible nodes. Copies are plain `Node`s (existing type) safe to read without locks.
- `Result.Roots` = `Snapshot(FileFloor)` at completion. Cancel → `Result()` returns partial result + `context.Canceled`.
- Multiple roots scanned sequentially; snapshot includes finished roots and the one in progress.
- Everything else unchanged (hard-link dedupe, devices, excludes, errors).

### tree

- `Tree` gains path-keyed expansion: `ExpandedPaths() map[string]bool` and `Rebuild(roots, filter, sort, expanded)`; roots not in the map start expanded, collapse is recorded as false.
- While scanning, Search restarts (instead of cancelling) if the form now differs from the running search.

### ui

- `app.go`: single screen, panes, focus, scan lifecycle (start/cancel/tick), filter re-application.
- `form.go`: custom form model.
- `treeview.go`: renders into given width/height (no header/footer of its own beyond tree rows); cursor restore by path.
- `scanning.go`: removed (status folded into results header).

## Testing

- scan: existing tests via `Scan` wrapper; new: snapshot during scan returns consistent partial sizes (parent ≥ sum of children); snapshot min pruning; cancel returns partial result.
- tree: expansion survives rebuild by path.
- ui: form field navigation, button label by state, enter starts scan, filter edit re-filters without rescan, min below floor → stale hint, tab/esc focus, narrow width shows one pane, view fits window.
