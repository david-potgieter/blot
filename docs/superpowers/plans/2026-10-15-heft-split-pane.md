# heft split-pane v2 Implementation Plan

> Executed inline (controller session), review at end. Spec: `docs/superpowers/specs/2026-10-15-heft-split-pane-design.md`.

**Goal:** One split screen: form (left) with Search/Cancel button, live-streaming results (right), instant min/type re-filtering.

**Architecture:** `scan` becomes a background `*Scan` with atomic ancestor-propagated sizes and cheap min-pruned snapshots. `tree` keeps expansion by path so rebuilds every 250ms preserve state. `ui` drops huh for a bubbles/textinput form and composes two bordered panes with lipgloss.

## Tasks

- [ ] **1. scan: background Scan + snapshots** — `internal/scan/scan.go`, `scan_test.go`
  - internal `live` node (atomic size, mutex-guarded children); file/dir usage added to node + all ancestors on discovery; remove `sumDirs`.
  - `Start`, `Progress`, `Snapshot(min)`, `Done`, `Result`; `Scan` wrapper keeps existing tests green.
  - New tests: snapshot min pruning; mid-scan snapshot consistency (dir size ≥ sum of children); cancel → partial result + `context.Canceled`.
- [ ] **2. tree: expansion by path** — `internal/tree/tree.go`, `tree_test.go`
  - `NewWithExpanded(roots, f, sort, expanded map[string]bool)`; `ExpandedPaths()`; `New` = roots expanded.
  - Test: rebuild from a fresh copy of nodes keeps expanded paths.
- [ ] **3. ui: form model** — `internal/ui/form.go`, `form_test.go`
  - textinput Folders/Min, Show selector, button; tab/shift+tab/up/down wrap; enter → `submitMsg`; inline errors; `values()`; `scanning` flag flips button label.
- [ ] **4. ui: tree view in a box** — `internal/ui/treeview.go`, `treeview_test.go`
  - `View(w, h)` renders rows only into given size; `setTree(t)` swaps tree preserving cursor by path; drop own header/footer/help.
- [ ] **5. ui: app split screen** — `internal/ui/app.go`, `app_test.go`, delete `scanning.go`, `keys.go` updates, `styles.go` borders
  - panes, focus, narrow mode, scan lifecycle (start/cancel/250ms tick/done), live filter re-apply, stale hint, header line, help line.
- [ ] **6. cmd + docs** — `cmd/heft/main.go` (no change to flags; `--no-form` → focus results), `README.md` (new layout/keys).
- [ ] **7. Verify** — gofmt, vet, `go test -race ./...`, final review subagent.
