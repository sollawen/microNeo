
## Configuration

microNeo uses `$XDG_CONFIG_HOME/microNeo/` (default `~/.config/microNeo/`) as its configuration directory.

- **Colorscheme** — Markdown rendering colors can be customized via your color scheme file. Built-in color schemes (`darcula`, `gruvbox-tc`, …) include these definitions. Custom schemes go in `~/.config/microNeo/colorschemes/`.
- **`settings.json`** — If you see garbled text on mouse click (common on Linux), set clipboard mode to `terminal`:

    ```json
    {
      "clipboard": "terminal"
    }
    ```

- **Font** — A Nerd Font or similar powerline-compatible font is recommended. If the status separator `` looks broken, change `status-separator` in `~/.config/microNeo/settings.json` (e.g., to `│`).
