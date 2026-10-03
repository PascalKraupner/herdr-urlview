# herdr-urlview

Open or copy a URL from the pane you're looking at in [herdr](https://herdr.dev).
Like tmux-urlview, with an fzf picker. Links appear newest first, without duplicates.

## Install

Requires herdr **0.9.3+**, Git, curl, tar, and [fzf](https://github.com/junegunn/fzf).
Linux and macOS are supported, on Intel/AMD and ARM64.

```bash
herdr plugin install PascalKraupner/herdr-urlview
```

Installation downloads a prebuilt binary and checks its SHA-256 checksum.
If no release asset is available, it builds from source with Go 1.26+.

Add this to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+u"
type = "shell"
command = "herdr plugin pane open --plugin urlview --entrypoint pick"
```

Reload the config with `herdr server reload-config`, then press your prefix and
`u`. Type to filter the list.

| Key | Action |
| --- | --- |
| Enter | Open in your default browser |
| Ctrl-Y | Copy to the clipboard |
| ↑ / ↓ or Ctrl-K / Ctrl-J | Move through links |
| Esc | Close |

Opening uses `xdg-open` or `gio` on Linux and `open` on macOS.
Copying uses `wl-copy` on Wayland, `xclip` on X11, or `pbcopy` on macOS.

## Wrapped links and history

The picker reads herdr's unwrapped terminal buffer, so a URL that wraps onto
several terminal rows stays one link. ANSI styling is removed before extraction.
Actual newlines remain boundaries; the plugin does not guess whether separate
lines of text were meant to be joined.

In a normal shell, it scans up to 1,000 recent rows. In an alternate-screen app
such as a coding agent, only the current buffer is available. Opening the picker
does not wheel-scroll the agent to fetch older output.

Set `HERDR_URLVIEW_LINES` to scan fewer rows. The default is `1000`, herdr's
maximum. Supported prefixes are `http://`, `https://`, `ftp://`, `file://`, and
`www.`. Trailing prose punctuation is removed; balanced parentheses are kept.

## Update or remove

Re-run the install command to update, or pin a release with `--ref v0.2.0`.

```bash
herdr plugin uninstall urlview
```

## Development

```bash
go test ./...
go build -o bin/herdr-urlview .
herdr plugin link .
```

Run the wrapped-link integration test with
`HERDR_TEST_BIN="$(command -v herdr)" go test -v -run TestLive`.
It starts an isolated test server.

Tested against herdr 0.9.3. Reports with sample output and the herdr version are
welcome in [Issues](https://github.com/PascalKraupner/herdr-urlview/issues).

## License

MIT
