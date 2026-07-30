# herdr-urlview

Pick a URL out of a [herdr](https://herdr.dev) pane and open it. Same idea as
[tmux-urlview](https://github.com/tmux-plugins/tmux-urlview), independently
implemented against herdr's socket API and sharing no code with it. Where
tmux-urlview shells out to `urlview` or `extract_url`, this does the extraction
itself and uses `fzf` for the picker.

Reads the invoking pane's scrollback through herdr's socket API, extracts URLs,
and offers them in an `fzf` picker, newest first.

| Key | Action |
|---|---|
| `enter` | open in the default browser |
| `ctrl-y` | copy to the clipboard instead |
| `esc` | cancel |

## Install

As a herdr plugin:

```bash
herdr plugin install PascalKraupner/herdr-urlview
```

Or as a plain keybinding, without the plugin system. Put `bin/herdr-urlview`
somewhere on `PATH` and add to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+u"
type = "popup"
command = "herdr-urlview"
width = "80%"
height = "60%"
```

## Requirements

- `fzf`
- `xdg-open` or `gio` on Linux, `open` on macOS
- `wl-copy`, `xclip` or `pbcopy` for the copy binding
- nothing else; it is a static Go binary

## Configuration

`HERDR_URLVIEW_LINES` sets how much scrollback to scan. Default `5000`.

## Notes

URL matching is deliberately conservative. Terminals wrap and decorate output,
so a more permissive pattern produces more false positives than real links it
recovers. Trailing prose punctuation is stripped, but a closing parenthesis is
kept when the URL has an unmatched opening one, so
`https://en.wikipedia.org/wiki/Foo_(bar)` survives.

## License

MIT
