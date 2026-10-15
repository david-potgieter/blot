# heft

Find big files and folders on your Mac, in your terminal.

`heft` scans folders in parallel and shows everything over a size threshold as a navigable tree.
Built with [Bubble Tea](https://charm.land), [Bubbles](https://charm.land), [Lip Gloss](https://charm.land) and [Huh](https://charm.land).

## Install

```sh
go install github.com/david-potgieter/heft/cmd/heft@latest
```

## Usage

```sh
heft                                  # opens a form
heft ~/Work --min 500MB               # prefill the form
heft ~/Work ~/Downloads --min 1GB --type files --no-form
```

| flag | default | |
|---|---|---|
| `--min` | `100MB` | minimum size (`500MB`, `1GiB`, …) |
| `--type` | `all` | `all`, `dirs` or `files` |
| `--no-form` | `false` | scan immediately |
| `--version` | | print version |

### Keys

| key | action |
|---|---|
| `↑`/`k` `↓`/`j` | move |
| `→`/`l`/`enter` | expand |
| `←`/`h` | collapse / go to parent |
| `g` / `G` | top / bottom |
| `s` | sort by size or name |
| `o` | reveal in Finder |
| `r` | rescan |
| `n` | new search |
| `?` | help |
| `q` / `ctrl+c` | quit |

### Notes

- Sizes are actual disk usage. Hard links count once; symlinks aren't followed; other volumes are skipped (scanning `/` includes the Data volume).
- Skipped by default: `/System`, `/Volumes`, `/dev`, swap, iCloud Drive, `~/Library/CloudStorage` (Dropbox, OneDrive…) and the Trash.
- macOS protects some folders. Without **Full Disk Access** for your terminal (System Settings → Privacy & Security), scanning `~` may trigger several permission prompts and skip protected folders.

## License

MIT
