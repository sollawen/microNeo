# <img src="/microNeo/assets/microNeo-logo-mark.svg" style="width:48px;height:auto" alt="microNeo logo" align="absmiddle"/> microNeo

**The only terminal Markdown editor that renders and edits in the same window.**

Every Markdown editor splits your screen — source left, preview right.
Terminal screens aren't wide to begin with. **microNeo renders and edits in the same window.**

- Click anywhere to edit the source
- See the result instantly, no split panes
- Works great as `$EDITOR` for `Claude Code`, `Yazi`, and friends

**Also a full text editor.** Like other editors, microNeo supports syntax highlighting for 100+ languages (Python, Go, Rust, JavaScript, C, HTML, JSON, YAML, …), mouse support, multiple cursors, and Lua plugins. Markdown rendering is the bonus on top, automatically applied to `.md` / `.markdown` files.

---

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/install.sh | sh
```

---

## Why microNeo

|                          | microNeo | Micro / nano | glow / leaf | vim + plugins | GUI Editors |
| ------------------------ | :------: | :----------: | :---------: | :-----------: | :---------: |
| Editable                 |    ✓     |      ✓       |      ✗      |       ✓       |      ✓      |
| Markdown Rendering       |    ✓     |      ✗       |      ✓      |       ✓       |      ✓      |
| Same Interface (no split) |   ✓    |      -       |      -      |      ✗ (split) |   ✗ (split) |
| Low Learning Curve       |    ✓     |      ✓       |      ✓      |       ✗       |      ✓      |

**microNeo = Micro's editing + Glow's rendering, in one window.**

---

## Usage

```bash
# Open any file
microneo README.md
```

### Set as Default Editor

```bash
export EDITOR=microneo
```

Works seamlessly with `Claude Code`, `Yazi`, and other tools that respect `$EDITOR`.

---

## Hotkeys

| Action        | Shortcut   | Action        | Shortcut   |
| ------------- | ---------- | ------------- | ---------- |
| Save          | `Ctrl-S`   | Undo          | `Ctrl-Z`   |
| Quit          | `Ctrl-Q`   | Search        | `Ctrl-F`   |
| Command Mode  | `Ctrl-E`   |               |            |

Press `Ctrl-E` and type `help` for more commands.

---

## Configuration

microNeo uses `$XDG_CONFIG_HOME/microNeo/` (default `~/.config/microNeo/`) for config.

- **Colorscheme** — Markdown rendering colors can be customized via your color scheme file. Built-in color schemes (`darcula`, `gruvbox-tc`, …) include these definitions. Custom schemes go in `~/.config/microNeo/colorschemes/`.
- **`settings.json`** — If you see garbled text on mouse click (common on Linux), set clipboard mode to `terminal`:

    ```json
    {
      "clipboard": "terminal"
    }
    ```

- **Font** — A Nerd Font or similar powerline-compatible font is recommended. If the status separator ` ` looks broken, change `status-separator` in `~/.config/microNeo/settings.json` (e.g., to `│`).

---

## FAQ

### What is the best Markdown editor for the terminal?

**microNeo** is a terminal Markdown editor that renders and edits in the same window — no split panes, no plugin setup, single Go binary. Unlike [Glow](https://github.com/charmbracelet/glow) (read-only) or vim with markdown plugins (steep learning curve), microNeo combines live Markdown rendering with full editing in one TUI window.

### Is there a Markdown editor with live preview that doesn't split the screen?

Yes — microNeo is the only terminal Markdown editor that renders and edits in the same window. You see the formatted Markdown by default; click anywhere to edit the source, and the preview updates instantly. No source/preview split, no tab switching, no detached preview window.

### Can I use microNeo to edit code and config files, or only Markdown?

Yes — microNeo is a full terminal text editor, not just a Markdown viewer. Like [Micro](https://github.com/zyedidia/micro), it supports syntax highlighting for 100+ languages (Python, Go, Rust, JavaScript, C, HTML, JSON, YAML, TOML, and more), mouse support, multiple cursors, and Lua plugins. Markdown rendering is the bonus that activates automatically for `.md` / `.markdown` files.

### How do I preview Markdown in the terminal?

Install microNeo, open any `.md` file with `microneo README.md`. Headings, tables, code blocks, lists, and links are rendered inline. Click to edit, save with `Ctrl-S`.

### microNeo vs Glow — what's the difference?

[Glow](https://github.com/charmbracelet/glow) is a read-only Markdown viewer (think `$PAGER`) — you can scroll and search but not edit. **microNeo is a full editor**: it renders formatted output AND lets you click anywhere to edit the source.

### How is microNeo different from Micro?

[Micro](https://github.com/zyedidia/micro) is a general-purpose terminal text editor with syntax highlighting and mouse support. microNeo keeps all of that and adds automatic Markdown rendering: opening a `.md` file shows formatted headings, tables, and code blocks inline instead of raw markup. The same window is used for both viewing and editing — click to switch.

### How do I use microNeo as the editor for Claude Code or opencode?

microNeo works with any tool that respects `$EDITOR` — including [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and [opencode](https://github.com/sst/opencode). Set it as your default:

```bash
export EDITOR=microneo
```

Add this to your shell profile (`~/.bashrc`, `~/.zshrc`, …) and reload.

### Does microNeo support custom themes and configuration?

Yes. microNeo uses `$XDG_CONFIG_HOME/microNeo/` (default `~/.config/microNeo/`). You can:

- Customize Markdown rendering colors via your color scheme
- Set clipboard mode in `settings.json` (fixes garbled click on Linux)
- Change the status separator for non-Nerd-Font terminals
- Bind custom hotkeys via `bindings.json`

---

## Relationship with Micro

microNeo originated from a fork of [Micro](https://github.com/micro-editor/micro) and is **no longer a fork** — the GitHub relationship was officially severed. The codebase still inherits from Micro's editor architecture (zero dependencies, intuitive operation, Lua plugins, mouse support) and adds a Markdown rendering layer on top.

microNeo is now developed independently, with the goal of becoming the best Markdown experience in the terminal.

---

## License

[MIT](https://github.com/sollawen/microNeo/blob/master/LICENSE)