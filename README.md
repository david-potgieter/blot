# blōt

*blōt (say "bloat"): find what's bloating your disk, in your terminal.*

`blot` scans folders in parallel and shows everything over a size you choose as a tree you can
browse. Results stream in while it scans. macOS only.

## Install

```sh
go install github.com/david-potgieter/blot/cmd/blot@latest
```

Make sure `~/go/bin` is on your `PATH`.

## How to use

1. Run `blot`.
2. In **Search** (left), type the folders to scan, e.g. `~/Work, ~/Downloads`. `tab` completes folder names.
3. Set **Min size** (e.g. `500MB`) and pick what to **Show** with `←`/`→`: files & folders, folders, or files.
4. Press `enter` to search. Focus jumps to **Results** (right) and the tree fills in as the scan runs.
5. Browse with the arrow keys. Press `o` to reveal an item in Finder, and delete it there.

While results are showing, changing Min size or Show re-filters straight away, with no rescan.
**Bin** (bottom left) shows what's in your Trash, biggest first.

Quit with `q`. Next time, plain `blot` reopens exactly where you left off; press `r` to rescan.

## Examples

```sh
blot ~ --min 1GB --type dirs --no-form       # biggest folders in your home, scan straight away
blot ~/Downloads --min 200MB --type files    # big files in Downloads, form prefilled
blot ~/Work ~/Code --min 500MB               # several folders at once
```

| flag | default | |
|---|---|---|
| `--min` | `100MB` | minimum size (`500MB`, `1GiB`, …) |
| `--type` | `all` | `all`, `dirs` or `files` |
| `--no-form` | off | start scanning immediately |
| `--version` | | print the version |

Any path or flag starts a fresh search instead of reopening the last one.

## Keys

| key | where | action |
|---|---|---|
| `tab` / `shift+tab` | anywhere | next / previous pane (in Folders, `tab` first completes a name) |
| `↑` `↓` | Search | previous / next field |
| `←` `→` | Search | change Show |
| `enter` | Search | Search, or Cancel while scanning |
| `↑`/`k` `↓`/`j` | Results, Bin | move |
| `→`/`l` `←`/`h` | Results | expand / collapse |
| `g` / `G` | Results | top / bottom |
| `s` | Results | sort by size or name |
| `o` | Results, Bin | reveal in Finder |
| `r` | Results | rescan (this folder or everything) |
| `q` / `ctrl+c` | anywhere (`ctrl+c` in Search) | quit |

## Good to know

- **Full Disk Access:** macOS hides some folders from your terminal. To scan them without permission prompts, add your terminal under System Settings → Privacy & Security → Full Disk Access.
- **Sizes** are real disk usage. Hard links count once, symlinks aren't followed, and other volumes are skipped.
- **Skipped:** `/System`, `/Volumes`, iCloud Drive, `~/Library/CloudStorage` (Dropbox, OneDrive…) and the Trash.
- **Going smaller:** a scan keeps everything down to 1MB (or your min, if that was lower). To show anything smaller, press Search again.
- **Commas:** write a comma inside a folder name as `\,` (and a backslash as `\\`). Completion does this for you.

## License

MIT
