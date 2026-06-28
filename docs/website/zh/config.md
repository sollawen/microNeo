
## 配置

microNeo 使用 `$XDG_CONFIG_HOME/microNeo/`（默认 `~/.config/microNeo/`）作为配置目录。

- **配色方案** — Markdown 渲染的颜色可以通过配色方案文件定制。内置配色（`darcula`、`gruvbox-tc` ……）已包含这些定义。自定义方案放在 `~/.config/microNeo/colorschemes/` 下。
- **`settings.json`** — 如果鼠标点击出现乱码（Linux 上常见），把剪贴板模式设为 `terminal`：

    ```json
    {
      "clipboard": "terminal"
    }
    ```

- **字体** — 推荐使用 Nerd Font 或其他兼容 powerline 的字体。如果状态分隔符 `` 显示异常，可以在 `~/.config/microNeo/settings.json` 中修改 `status-separator`（例如改成 `│`）。
