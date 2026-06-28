
## FAQ

### What is the best Markdown editor for the terminal?

**microNeo** is a terminal Markdown editor that renders and edits Markdown in the same window — no split panes, no plugin setup, single Go binary. Unlike [Glow](https://github.com/charmbracelet/glow) (read-only) or vim with markdown plugins (steep learning curve), microNeo combines live rendering with full editing in one TUI window.

### Is there a Markdown editor with live preview that doesn't split the screen?

Yes — microNeo is the only terminal Markdown editor that renders and edits in the same window. You see the formatted Markdown by default; click anywhere to edit the source, and the preview updates instantly. No source/preview split, no tab switching, no detached preview window.

### Can microNeo only edit Markdown?

No — microNeo is a full-featured terminal text editor, not just a Markdown viewer. Like [Micro](https://github.com/zyedidia/micro), it supports syntax highlighting for 100+ languages (Python, Go, Rust, JavaScript, C, HTML, JSON, YAML, TOML, …), mouse support, multiple cursors, and Lua plugins. Markdown rendering is the bonus that automatically applies to `.md` / `.markdown` files.

### How do I preview Markdown in the terminal?

After installing microNeo, run `microneo README.md` to open any `.md` file. Headings, tables, code blocks, lists, and links are rendered inline. Click to edit, `Ctrl-S` to save.

### What's the difference between microNeo and Glow?

[Glow](https://github.com/charmbracelet/glow) is a read-only Markdown viewer (like `$PAGER`) — you can scroll and search, but not edit. **microNeo is a full editor**: it renders the formatted result AND lets you click anywhere to edit the source.

### What's the difference between microNeo and Micro?

[Micro](https://github.com/zyedidia/micro) is a general-purpose terminal text editor with syntax highlighting and mouse support. microNeo keeps all of that and adds automatic Markdown rendering: opening a `.md` file shows formatted headings, tables, and code blocks instead of raw markup. Viewing and editing share the same window — click to switch.

### How do I set microNeo as the editor for Claude Code or opencode?

microNeo is compatible with any tool that respects `$EDITOR` — including [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and [opencode](https://github.com/sst/opencode). Set it as default:

```bash
export EDITOR=microneo
```

Add it to your shell profile (`~/.bashrc`, `~/.zshrc`, …) and reload.

### Does microNeo support custom themes and configuration?

Yes. microNeo uses `$XDG_CONFIG_HOME/microNeo/` (default `~/.config/microNeo/`). You can:

- Customize Markdown rendering colors via your color scheme
- Set clipboard mode in `settings.json` (fixes garbled click on Linux)
- Change the status separator for non-Nerd-Font terminals
- Bind custom hotkeys via `bindings.json`
