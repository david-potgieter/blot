# heft — design (v1: find only)

Go TUI to find files and folders on macOS over a size threshold. Built on Charm libs. Open-source ready (MIT).

Module: `github.com/david-potgieter/heft` · binary: `heft` · Go 1.25

## Scope v1

- Input roots, min size, type (all | dirs | files).
- Scan, then browse results as a navigable tree.
- **No deletion** (next iteration). No staleness filter, no presets, no cache.

## Flow

1. **Form** (`charmbracelet/huh`): root paths (comma-separated, `~` expanded, default `~`), min size (e.g. `500MB`, default `100MB`), type (default `all`). CLI flags prefill fields: `heft [paths...] --min 500MB --type dirs`. `--no-form` skips form when flags/args given.
2. **Scanning**: `bubbles/spinner`, live counters (files, dirs, bytes, errors), current path (truncated). `esc`/`ctrl+c` cancels via `context`.
3. **Tree**: filtered results. Each root is a top-level node.

## Tree screen

Row: `▸/▾  size  bar  name  modified`.
- size: humanized (SI, `go-humanize`).
- bar: lipgloss block bar, width proportional to size / parent size.
- modified: dir/file mtime, relative ("3 weeks ago").

Keys (`bubbles/key` + `bubbles/help`):

| key | action |
|---|---|
| ↑/k ↓/j | move |
| →/l/enter | expand |
| ←/h | collapse, or jump to parent |
| g / G | top / bottom |
| s | toggle sort size ↔ name |
| o | reveal in Finder (`open -R <path>`) |
| r | rescan same params |
| n | new search (back to form) |
| ? | full help |
| q / ctrl+c | quit |

Footer: total scanned size, match count, error count, short help. Viewport scrolls; layout respects window size (`tea.WindowSizeMsg`).

### Visibility rules (threshold T, type)

A node is **visible** if:
- `all`: node.Size ≥ T (dir or file).
- `dirs`: node is dir and Size ≥ T.
- `files`: node is file with Size ≥ T, **or** dir containing a visible file (context path; dir shown dimmed).

Roots always visible. Dirs default collapsed except roots expanded. Sort default size desc.

## Architecture

```
cmd/heft/main.go     flag parsing (std flag), builds config, runs tea.Program
internal/size/       Parse("500MB")→int64, Format(int64)→string
internal/scan/       Scan(ctx, roots, opts, progress chan) → []*Node; no UI deps
internal/tree/       pure: Visible(node, filter), Flatten(roots, expanded, sort) → []Row
internal/ui/         app model (state machine form→scanning→tree), sub-models
```

Units only depend downward: `ui → tree → scan`, `ui → size`. `scan` and `tree` have no Charm imports.

### scan

```go
type Node struct {
    Name     string
    Path     string
    IsDir    bool
    Size     int64     // disk usage (st_blocks*512), recursive for dirs
    ModTime  time.Time
    Parent   *Node
    Children []*Node   // dirs: all child dirs + files with Size >= FileFloor
}
type Options struct {
    FileFloor int64    // files below this are counted into parent, not stored
    Exclude   []string // absolute paths to skip
}
type Progress struct { Files, Dirs, Bytes, Errors int64; Current string }
```

- Walker: `charlievieth/fastwalk` (parallel). Nodes built per dir with mutex-protected parent linkage; sizes aggregated bottom-up after walk completes.
- `FileFloor` = min size from form (memory scales with dir count, not file count).
- Size via `syscall.Stat_t.Blocks * 512`.
- Hard links: files with `Nlink > 1` deduped by `(Dev, Ino)`; first seen counts.
- Symlinks not followed (counted at their own lstat size).
- Stay on root's device (`Dev` compare); skip mount points.
- Default excludes: `/System`, `/Volumes`, `/dev`, `/private/var/vm`, `~/Library/Mobile Documents`, `~/.Trash`.
- Errors (EPERM, TCC): counted + kept in list (first 100); never fatal. Footer hints Full Disk Access when errors > 0.
- Progress emitted throttled (~10/s) via channel; UI reads with `tea.Cmd` loop.
- Cancellation via `ctx`.

### tree

- `Filter{Min int64; Type Type}`; `Visible(n, f) bool`.
- `Flatten(roots, expanded map[*Node]bool, f, sort) []Row` where `Row{Node, Depth, Expanded, HasChildren, Context bool}`.
- Pure functions, fully unit-tested.

### ui

- `app` model holds state enum + sub-models: `formModel` (huh), `scanModel`, `treeModel`.
- Styles in one `styles.go` (lipgloss), adaptive colors for light/dark.

## Errors

- Invalid size string → huh field validation error.
- Nonexistent root → form validation error.
- All scan errors non-fatal; shown as count.

## Testing

- `size`: table tests parse/format.
- `scan`: temp-dir fixtures (nested dirs, files of known size, hardlink, symlink); assert sizes, pruning by FileFloor, dedupe, cancellation.
- `tree`: synthetic Node trees; visibility per type, flatten order, expand/collapse, sort.
- `ui`: `charmbracelet/x/exp/teatest` for tree navigation golden output.

## Open source setup

- MIT `LICENSE` (David Potgieter), `README.md` (install, usage, keys, Full Disk Access note).
- `.github/workflows/ci.yml`: `go vet`, `go test -race`, macOS runner.
- `.goreleaser.yaml`: darwin amd64/arm64 builds. Homebrew tap later.
- `.gitignore`.

## Later (not v1)

Trash/delete, staleness filter, name/glob presets (node_modules etc.), scan cache, JSON/headless output.
