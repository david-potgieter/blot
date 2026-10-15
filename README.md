# heft

Find big files and folders on your Mac, in your terminal.

`heft` scans folders in parallel and shows everything over a size threshold as a navigable tree.
The search form sits on the left and results stream in on the right as the scan runs; changing
the minimum size or type re-filters instantly, without rescanning.
Built with [Bubble Tea](https://charm.land), [Bubbles](https://charm.land) and [Lip Gloss](https://charm.land).

## Install

```sh
go install github.com/david-potgieter/heft/cmd/heft@latest
```

## Usage

```sh
heft                                  # empty results, form focused
heft ~/Work --min 500MB               # prefill the form, press enter to search
heft ~/Work ~/Downloads --min 1GB --type files --no-form
```

| flag | default | |
|---|---|---|
| `--min` | `100MB` | minimum size (`500MB`, `1GiB`, …) |
| `--type` | `all` | `all`, `dirs` or `files` |
| `--no-form` | `false` | scan immediately |
| `--version` | | print version |

### Keys

Search pane (left):

| key | action |
|---|---|
| `tab`/`↓`, `shift+tab`/`↑` | next / previous field |
| `←`/`→` | change *Show* (files & folders, folders, files) |
| `enter` | press the button: **Search**, or **Cancel** while scanning |
| `esc` | go to results |
| `ctrl+c` | quit |

Results pane (right):

| key | action |
|---|---|
| `↑`/`k` `↓`/`j` | move |
| `→`/`l`/`enter` | expand |
| `←`/`h` | collapse / go to parent |
| `g` / `G` | top / bottom |
| `s` | sort by size or name |
| `o` | reveal in Finder |
| `r` | rescan |
| `tab` | go to search |
| `q` / `ctrl+c` | quit |

Lowering the minimum size below what was scanned (1MB, or your minimum if smaller), or changing
folders, needs a rescan; the results header says so. Terminals narrower than 90 columns show one
pane at a time.

### Notes

- Sizes are actual disk usage. Hard links count once; symlinks aren't followed; other volumes are skipped (scanning `/` includes the Data volume).
- Skipped by default: `/System`, `/Volumes`, `/dev`, swap, iCloud Drive, `~/Library/CloudStorage` (Dropbox, OneDrive…) and the Trash.
- macOS protects some folders. Without **Full Disk Access** for your terminal (System Settings → Privacy & Security), scanning `~` may trigger several permission prompts and skip protected folders.

## License

MIT
