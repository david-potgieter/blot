# blōt

*blōt (say "bloat"): find what's bloating your disk, in your terminal.*

`blot` scans folders in parallel and shows everything over a size threshold as a navigable tree.
The search form sits on the left and results stream in on the right as the scan runs; changing
the minimum size or type re-filters instantly, without rescanning.
Built with [Bubble Tea](https://charm.land), [Bubbles](https://charm.land) and [Lip Gloss](https://charm.land).

## Install

```sh
go install github.com/david-potgieter/blot/cmd/blot@latest
```

## Usage

```sh
blot                                  # back where you left off (or an empty form the first time)
blot ~/Work --min 500MB               # prefill the form, press enter to search
blot ~/Work ~/Downloads --min 1GB --type files --no-form
```

| flag | default | |
|---|---|---|
| `--min` | `100MB` | minimum size (`500MB`, `1GiB`, …) |
| `--type` | `all` | `all`, `dirs` or `files` |
| `--no-form` | `false` | scan immediately |
| `--version` | | print version |

### Keys

`tab` / `shift+tab` move between panes: Search, Bin, Results.

Search pane (top left):

| key | action |
|---|---|
| `↓` / `↑` | next / previous field |
| `tab` | in *Folders*: complete the folder name being typed (`ctrl+n`/`ctrl+p` for other matches) |
| `←`/`→` | change *Show* (files & folders, folders, files) |
| `enter` | press the button: **Search**, or **Cancel** while scanning |
| `esc` | go to results |
| `ctrl+c` | quit |

*Folders* takes several paths separated by commas. For a comma inside a folder name type `\,`
(and `\\` for a backslash); completion and paths given on the command line do this for you.

Results pane (right):

| key | action |
|---|---|
| `↑`/`k` `↓`/`j` | move |
| `→`/`l`/`enter` | expand |
| `←`/`h` | collapse / go to parent |
| `g` / `G` | top / bottom |
| `s` | sort by size or name |
| `o` | reveal in Finder |
| `r` | rescan from the focused folder, or everything |
| `q` / `ctrl+c` | quit |

Bin pane (bottom left) shows what's in the Trash, biggest first:

| key | action |
|---|---|
| `↑`/`k` `↓`/`j` | move |
| `o` | reveal in Finder |
| `r` | measure again |
| `enter` | without Full Disk Access: offer to open System Settings |
| `q` / `ctrl+c` | quit |

Lowering the minimum size below what was scanned (1MB, or your minimum if smaller), or changing
folders, needs a rescan; the results header says so. Terminals narrower than 90 columns show one
pane at a time.

### Sessions

Quitting saves the whole view (search, results, open folders, cursor) to
`~/Library/Caches/blot/`. Running plain `blot` again shows it as you left it, marked
"saved … ago", without rescanning; press `r` or Search to rescan. Passing any paths or
flags starts fresh.

### Notes

- Sizes are actual disk usage. Hard links count once; symlinks aren't followed; other volumes are skipped (scanning `/` includes the Data volume).
- Skipped by default: `/System`, `/Volumes`, `/dev`, swap, iCloud Drive, `~/Library/CloudStorage` (Dropbox, OneDrive…) and the Trash.
- macOS protects some folders. Without **Full Disk Access** for your terminal (System Settings → Privacy & Security), scanning `~` may trigger several permission prompts and skip protected folders.

## License

MIT
