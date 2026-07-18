# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

**Added**

- Finder supports renaming files and directories with `r`.
- Finder supports permanently deleting files and non-empty directories with `d`, with a confirmation prompt before removal.
- `MsgDialog` provides a multi-line message modal.
- `InputDialog` provides a single-line text input modal.
- `ConfirmDialog` provides an OK/Cancel confirmation modal for destructive actions.

**Refactor**

- 把键名翻译能力从 `action` 包私有下沉到 `config` 包公共，消除 `keyResolver` 注入机制。叶子包（dialog/finder）现在能直接调用 `config.KeyName` 自己闭环处理键盘和用户自定义键位。新增 `internal/config/keyname.go`。
- 把 modal 浮窗组件从 `internal/action/` 迁出，创建独立的 `internal/dialog/` 包。结构变化：
	 - 新增 `internal/dialog/` 包（`frame.go`、`select.go`），承载 `FloatFrame` 容器与 `SelectDialog` 选择器，以及共享的 `Rect/Pos/Size/FloatOpenSpec` 契约。包内自初始化 `TheFloatFrame`，不再依赖 action 全局变量。
	 - 删除 `internal/action/floatframe.go`（~303 行）与 `internal/action/selectdialog.go`（~235 行），原 `action.TheFloatFrame` / `action.Rect` 等符号移至 `dialog.TheFloatFrame` / `dialog.Rect`。
	 - 引用点迁移：`cmd/micro/micro.go`（Display / HandleEvent 路由）与 `internal/action/{command_neo.go,notepane.go}`（`:theme` 与 NotePane receiver 选择）改为引用 `dialog` 包。
	 - `internal/action/globals.go` 删除 `TheFloatFrame` 初始化，交由 dialog 包自理。

## [1.1.18] - 2026-07-18

**Fixed**

- 修复中文文件名 tab 点击检测不准确的问题（多宽字符导致点击区域偏移）

**Changed**

- `Ctrl-t` 现在创建新 tab 页（携带空 buffer），不再打开水平分割。
- 在目录树里面导航切换目录后，即使没有编辑文件直接ctrl-q退出，也能cd到新的目录里

**Refactor**

- 把文件选择器从「全局 FloatFrame 浮窗」重写为「pane-local overlay」，并把目录导航 / git 查询 / 字符串工具独立成新的 `internal/finder` package。结构变化（核心）：
	 - 新增 `internal/finder` package（Session + state + git + strutil + model），从 action 包下沉出去，自画边框、自截事件、自算布局，不再依赖全局 FloatFrame。原 `fileselector.go`(1179) / `fileselector_git.go` / `filemanager.go` 全部删除（净减约 2000 行），git 查询与字符串工具随包迁入 `internal/finder`。
	 - 每个 BufPane 持有私有 `finder.Session`（`h.finder`）。事件路由改为：`HandleEvent` 在最顶部 `IsOpen()` 时整段转发给 finder（modal）；`Display()` 在 `BWindow.Display()` 之后叠加 `finder.Display()`。失焦兜底：`BufPane.SetActive(false)` 调 `onOwnerBlur`，覆盖「点别的 pane 切焦点」这条绕过 modal return 的路径。
	 - birth 检测从「imperative 接管」改为「declarative 自动触发」：每个 pane 在 `newBufPane` 被授予一次性 `pendingBirth` 许可，`BufPane.Display` 首帧消费（EventResize 处理完之后，避开 finder 的 resize 自关），按 noName 三条件判定后 `OpenFinder`。删除全部 spawn 包装（`neoAddTabAction` / `neoVSplit` / `neoHSplit` + 三个 Cmd 变体）与 `command_neo.go` 里对 `BufKeyActions`/`commands` 的 split/tab 覆盖，以及 `main.go` 里的 `OpenBirthSelectors` 调用。
	 - `action/fileops.go` 作为接线层：`OpenFinder` 统一三入口（Ctrl-o / noName 的 Ctrl-q / birth），`onFinderClose` 按 `Result.Reason` 一维分发（Picked→`OpenCmd` / Quit→`doQuit` / Esc·Resize→no-op）。
	 - shell cwd 上报：新增包级 `LastFinderCwd`，finder 因 Quit 关闭时由 `doQuit` 写入；`cmd/micro` `lastWorkingDir` 优先读它，让 `--cwd-file` 能跟到 finder 里导航到的目录。
	 - FloatFrame 保留（SelectPane 仍用），仅 gofmt 与 `AutoExpand=false` 注释更新（不再提已删除的 FileSelector）。


## [1.1.17] - 2026-07-15

**Fixed**

- In the file picker, files and subdirectories now keep their `U` / `I` git-status markers after navigating into an untracked or ignored directory (previously the markers disappeared below the top level of such directories).
- In multi-tab layouts, the active tab's highlight color no longer bleeds past `]` onto the trailing padding cells — the highlight now ends cleanly at the right bracket.

## [1.1.16] - 2026-07-15

**Changed**

- Tab switching is bound to `Alt-9` (previous, wrapping to last) and `Alt-0` (next, wrapping to first); `Ctrl+PageUp` / `Ctrl+PageDown` are no longer used for this.

## [1.1.15] - 2026-07-15

**Added**

- Resize the current pane with `Alt-=` (grow) and `Alt--` (shrink); each press snaps the split to the next of 25% / 50% / 75%. These keys can now be customized in `bindings.json`.
- `:big` promotes the active pane to its own fullscreen tab, moving it out of a split.
- `:small` demotes the active pane: from a 2-pane tab it moves the pane to a new tab; from a single-pane tab it pulls in a pane from another tab, or opens a fresh split if no other tabs exist.
- The active tab now shows a distinct highlight color in every colorscheme, making the current tab easy to spot at a glance (previously indistinguishable from inactive tabs in most themes).

**Changed**

- Each tab is limited to at most two panes; attempting a third split shows "already 2 panes in this tab".
- Pane resize keys are changed from `Alt-,` / `Alt-.` (previous tab) to `Alt-=` / `Alt--` (grow / shrink). These keys and `Ctrl-t` (horizontal split) can now be customized in `bindings.json`.
- `Alt-=` / `Alt--` now overflow at the size boundary instead of stopping: growing past 75% promotes the pane to a fullscreen tab (`:big`), and shrinking past 25% demotes it (`:small`). On a single-pane tab, `Alt--` triggers `:small` (absorb a pane from another tab, or open a fresh split) while `Alt-=` is a no-op.
- The s-dark tab bar now uses a dark background matching the status line, replacing the light gray that clashed with the dark theme.

## [1.1.14] - 2026-07-14

**Added**

- The file picker now accepts `q` as a quit shortcut, equivalent to `Ctrl-q`.

**Changed**

- `Ctrl-t` now opens a horizontal split (popping up the file picker) instead of creating a new tab.

**Fixed**

- The git `I` (ignored) indicator no longer incorrectly marks parent directories as ignored when only a nested subdirectory is ignored.

## [1.1.13] - 2026-07-13

**Added**

- Use microNeo in place of `cd`: with the new `--cwd-file <path>` flag, microNeo writes the directory of the file you last had open to a given file, so a shell wrapper can `cd` there on exit — a yazi `y()`-style behavior that turns microNeo into a `cd` replacement on any machine, including ones without yazi.

## [1.1.12] - 2026-07-11

**Added**

- Each row in the file picker now shows a right-aligned file size (human-readable, e.g. `12.3K`); directories are left blank.
- FileSelector marks git-ignored files and directories with an `I` indicator, so build outputs and tooling caches like `node_modules/` or `dist/` are easy to spot at a glance.

**Changed**

- The file picker's row layout was significantly reworked; the git status indicator is now consistently pinned to the right edge of each row instead of floating after the file name.
- The picker's bottom metadata line now includes file permissions alongside size and modified time.
- Optimized FileSelector code organization — all hotkey and command handlers moved to `command_neo.go`, making `filemanager.go` a pure executor that only opens selectors.

**Fixed**

- `Ctrl-q` on an empty pane in a too-narrow window now quits directly instead of hanging — the file selector can't fit, so it falls back to native quit.
- Shrinking the window while the file selector is open no longer force-quits the editor; it just closes the selector and returns you to editing.

## [1.1.11] - 2026-07-10

**Added**

- Per-pane file navigation: starting without a file, splitting a pane, or opening a new tab now opens a file picker at the parent pane's directory. Pressing `Ctrl-q` on an empty-born pane opens a picker offering files to switch into before closing. Startup, split, and tab workflows are unified under a single per-pane model.

**Changed**

- `Ctrl-o` no longer prompts to save before opening the file picker; the save prompt is deferred until you actually switch to a file, matching `:open`.
- In the quit selector, `Esc` cancels and returns to editing instead of being swallowed.

**Removed**

- All `Fn` function keys (F2/F3/F4/F7/F10) lose their default bindings — use `Ctrl-s` (save), `Ctrl-f` (find), `Ctrl-q` (quit). F-key identification is retained, so Fn keys can still be bound manually in settings.

**Fixed**

- `Ctrl-t` (new tab) now opens the file picker on the new pane, matching `:tab` and the split shortcuts.
- Quitting a modified noName pane no longer asks twice whether to save — the prompt appears once, at the actual point of discarding changes.
- Markdown files opened into a pane whose first file was not Markdown now render correctly (code blocks, headings, and table borders previously broke).

## [1.1.10] - 2026-07-09

**Added**

- New `--update-aibp` CLI flag: updates aibp extensions to the latest released version for AI agents that don't self-update (opencode, claude). Checks the npm registry for opencode and runs `claude plugin update` for claude; pi is skipped since it has its own in-app upgrade prompt. Prints progress to stdout and exits without opening the editor.

## [1.1.9] - 2026-07-08

**Added**

- New `:file` command opens a pane-local file picker to browse directories and open a file into the current pane — a visual alternative to `:open`.
- The file picker shows a breadcrumb path (Enter/← goes up a level, → enters a directory), marks directories, toggles dotfile visibility with `.`, and starts with the cursor on the current file.
- Git status indicators (`M`/`U`/`A`/`D`/`R`) appear next to file names when `diffgutter` is enabled, loaded in the background so the list is usable immediately.
- The file picker bottom line shows metadata for the selected entry, including directory child counts, file size, modified time, and symlink targets.
- `fileselectwidth` option (default `0.4`) controls the picker width as a fraction of the pane.

**Changed**

- `Ctrl-o` now opens the file picker directly, instead of prompting for a file name via the command line. It now behaves the same as the `:file` command; users who prefer the old prompt can rebind it to `OpenFile` in `bindings.json`.
- File picker visuals now better match directory/file context: breadcrumb rows use directory coloring, and long names are right-truncated while preserving extensions.
- Popups (theme picker, file picker) now close automatically when the terminal is resized, instead of leaving visual artifacts.

**Fixed**

- Git status indicators now also appear correctly when browsing into subdirectories in the file picker.
- microNeo's opencode receiver (`aibp-opencode`) loads again on opencode 1.17.15+: the plugin had stopped activating silently after opencode's bundled OpenTUI upgrade, so the AIBP name (e.g. `● Bravo`) disappeared and Alt-Enter deliveries to opencode were lost. Updating to `aibp-opencode` 1.0.5 (via `microneo --check-agent` or `opencode plugin aibp-opencode -g`) restores it. (aibp-opencode 1.0.5)

## [1.1.8] - 2026-07-05

**Fixed**

- Fenced code blocks that immediately follow a list, blockquote, or table now render correctly in place; previously the preceding structure overlapped and displaced the code block.
- Table rows containing an odd number of backticks (or other unmatched paired delimiters like `'`/`"`/`(`/`[`/`{`) no longer break table detection; the table used to be split at that row.

## [1.1.7] - 2026-07-04

**Fixed**

- Indented fenced code blocks (1-3 leading spaces before the ```` ``` ```` fence, common in AI-generated markdown) are now detected and rendered as code blocks instead of plain text.
- Tab characters in normal paragraphs and blockquotes are now expanded to align to the tab stop. Previously each tab rendered as a single column, so tab-indented Markdown text lost its alignment in the rendered view.
- Tab indentation inside blockquotes now aligns to the same absolute column as in edit mode, keeping the rendered and raw views consistent.

## [1.1.6] - 2026-07-03

这次写Claude的通信机制，是我开发microNeo以来最难受的一次
- Claude不是开源软件，好多api没有对外
- Claude对于使用第三方LLM的用户，额外的封锁了很多TUI的接口
- Claude像个贼一样的给中国区的用户埋设木马

这些种种都让我处在捏着鼻子写代码的状态中。我估计这是我最后一次为Claude写扩展程序了

**Added**

- Claude Code joins AIBP as the third AI agent receiver: microNeo can now deliver selections / cursor context to Claude Code, alongside the existing pi and opencode receivers. Install via the `microNeo-plugins` marketplace (`claude plugin install aibp-claude@microNeo-plugins`), or `claude --plugin-dir <path>` for dev iteration. (aibp-claude 1.0.1)
- `:check-agent` / `microneo --check-agent` now covers Claude Code too (in addition to pi / opencode): detects the plugin, prompts to add the marketplace + install if missing, validates protocol compatibility otherwise.

**Fixed**

- aibp-claude install flow for third-party LLM relay users (ccmm / other `ANTHROPIC_BASE_URL` proxies): marketplace install now works regardless of `ANTHROPIC_BASE_URL`, so relay users only need the env vars — no `--plugin-dir` flag required.

aibp-claude 1.0.1 also drops the always-on `/tmp/aibp-claude.log` diagnostic log that v1.0.0 carried over from the dev cycle.

## [1.1.5] - 2026-06-30

**Changed**

- AIBP selection context sent to AI agents now uses paired XML tags (e.g. `<selection ...> ... </selection>`) instead of single-sided markers. Removes ambiguity when selected text contains `<...>` strings (common in Markdown / HTML / config files), and shrinks each message by a few lines. (aibp-pi / aibp-opencode 1.0.4)

**Docs**

- README hero refreshed with an AI Partner intro and demo.
- Website gained a Changelog page and a custom 404 page.
- Repo root slimmed from 23 to 17 entries (moved `install.sh` to `tools/`, relocated `aibp-agents/` under `internal/aibp/`, etc.); no functional impact.

## [1.1.4] - 2026-06-28

**Added**

- `:theme` command: VS Code-style colorscheme selector popup. Replaces the legacy `set colorscheme <name>` flow — `↑/↓` browse, `Enter` picks, `Esc` cancels. The old command still works.
- New `default` 16-color ANSI colorscheme.

**Changed**

- Bundled colorschemes slimmed from 27 to 9 (kept the most representative; 18 redundant ones removed).
- SelectPane supports list scrolling (with `▲▼` overflow indicators) and caller-configured viewport height / wrap behavior.
- FloatFrame supports bottom-anchored popups via negative `anchor.Y`, so popups can snap to just above the status line.
- NotePane left edge now aligns with the main editor text area.

**Docs**

- Website: added Theme and Syntax Highlighting sections, full EN translation, restyled header.
- Language switcher replaced with a one-click `中文/English` chip.
- README: replaced screenshot with a live demo video, added Chinese-docs badge.

## [1.1.3] - 2026-06-26

**Changed**

- `:check-agent` TUI command migrated to `microneo --check-agent` CLI flag: agent extension self-heal (check / install / update) is now a shell command that prints progress to stdout and exits, rather than a TUI command blocking the editor loop. Run `microneo --check-agent` to verify and self-heal aibp-pi / aibp-opencode extensions for all installed AI agents. The flag works before config/screen init, so it can diagnose even when the config directory is corrupted.

**Fixed**

- npm-installed aibp-pi was misidentified as a source install: `piNpmAIBPVersion` transparently forwarded `aibp.ParseProtocol`'s three return values, so the `ok` (parse success) flag was silently repurposed as `isSource`. Any npm install with a parseable protocol got `isSource=true`, short-circuiting all version checks (no update / upgrade prompts ever fired for aibp-pi). Now explicitly drops `ok` and forces `isSource=false`, matching the already-correct `opencodeNpmAIBPVersion`.

## [1.1.2] - 2026-06-26

Second minor version after dev2: opencode joins AIBP as the second AI agent receiver available to microNeo; `:check-agent` now covers both pi and opencode; AIBP protocol number upgraded to `aibp-2.0` alongside the registry directory rename (major bump — see compatibility note in Changed).

**Added**

- opencode receiver (`aibp-opencode` npm package): microNeo can now deliver code selections / cursor context to opencode's current conversation, matching aibp-pi behavior. After the plugin loads, the AIBP-allocated name (e.g. `● Bravo`) is shown persistently at the bottom of opencode's TUI, making it easy to identify the current owner in multi-receiver setups.
- opencode auto-create-session delivery: when opencode has no active session, sending automatically creates a new session and writes the message into the prompt — no need to manually open a conversation first.
- `:check-agent` now covers opencode: in addition to pi, the command now checks opencode too. If not installed, it prompts to install opencode + the aibp-opencode extension; if installed, it validates protocol version compatibility.

**Changed**

- AIBP protocol number `aibp-1` → `aibp-2.0`: the registry directory was renamed from `microneo-agent-bridge-<UID>` to `aibp-<UID>`. This is an implementation-coupling change (both ends must upgrade in lockstep to keep talking), treated as a major bump by semver convention. **aibp-pi / aibp-opencode 1.0.1+ requires microNeo 1.1.2+; an old microNeo paired with a new agent = silently dropped messages.**
- `:check-agent` Install / Update flow split: pinned versions and source-tree installations are now handled correctly.
- Registry json gains an `agent` field: records which agent owns a name (`pi` / `opencode`), making it easy to tell ownership at a glance during troubleshooting. GC behavior is unchanged.

**Fixed**

- `aibp-opencode` self-install / upgrade on the opencode side no longer silently fails in pinned-version, pinned-cache, or `tui.json` mishandle scenarios.
- After the protocol number upgrade, the envelope V field was not refreshed in sync, causing messages to be silently dropped by the receiver.

## [1.1.1] - 2026-06-24

**Added**

- `InfoBarNow(msg)` helper: synchronous InfoBar refresh during blocking commands. `Ensure` now reports progress via `Reporter` callback — agent init shows `aibp-pi downloading.....` / `installed` / `ready` in real time instead of a frozen screen.

**Changed**

- `Ensure` signature takes a `Reporter` parameter. `CheckAibpCmd` simplified to one line: `_ = Ensure(PiEnsurer{}, InfoBarNow)`.

## [1.1.0] - 2026-06-23

dev2 分支合并到 master。这是 microNeo 第一个 minor 版本，引入 microNeo ↔ ai agent 通信的完整闭环：notePane 浮窗 + AIBP 协议 + pi 接收端（`aibp-agents/pi` npm 包）。从此 microNeo 不只是 Markdown 编辑器，而是 ai agent 的"前端外设"。

**Added**

- **AIBP 协议层**（`internal/aibp/`）：microNeo ↔ ai agent 通信的 LSP 式协议。注册表 = `$XDG_RUNTIME_DIR/microneo-agent-bridge-$UID/ai-*.json`；传输 = Unix socket + 逐行 JSON；line/col 1-based 对齐 LLM 工具链。协议版本号单一事实来源（`aibp.Protocol`）。
- **notePane 浮窗**（`internal/action/notepane.go`）：嵌入式 `*BufPane` + 白名单 bindings（约 80 个安全 action，从根上隔离 `Quit`/`Shell`/`OpenFile` 等危险 action）+ 独立 binding 树 + `BTScratch` 无文件 buffer（关闭即清空）。光标下方定位，主编辑器冻结。
- **`:check-agent` 命令**（`internal/action/command_neo.go`）：用户主动运行，检查本机是否装了 pi，没装就提示先装；装了但没 aibp-pi 就自动 `pi install npm:aibp-pi`；装了则校验协议版本兼容性（扩展过旧只提示不自动升，microNeo 过旧提示升级）。逻辑通过 `AgentEnsurer` 接口编排，未来接 opencode/claude 只新增 `ensure_<agent>.go` 即可。
- **`Alt-Enter` 打开 notePane**（主编辑器）：默认绑定，在主编辑器里按下时打开 notePane；在 notePane 内按下时则发送当前草稿 + 主编辑器上下文给当前 receiver 后关闭。**`Esc` 永远只关闭不发送**（TUI 约定）。
- **`Alt-i` 在 notePane 内切换 receiver**（`NotePaneSwitchReceiver`）：notePane 已开态下按下，调 `aibp.Discover()` 找当前可用 receiver，0 个 → InfoBar 报错，1 个 → 静默切到唯一那个，≥2 个 → 弹 SelectPane 列表让用户挑。只更新 `selectedReceiver` 字段不重建 buffer，**草稿不丢**。notePane 未开时 `alt-i` 静默 no-op。
- **SelectPane 通用浮窗**（D13）：notePane 内部切换 receiver 时弹出的列表选择浮窗。基于新 `FloatFrame` 框架，支持锚点自适应定位（靠近 notePane 上边框且自动展开方向）。上边框嵌入当前名字（`┌─name───┐`）。
- **pi 接收端**（`aibp-agents/pi/index.ts`，独立 npm 包 `aibp-pi`）：pi 启动时注册到 AIBP 注册表并监听 Unix socket；收到 microNeo 的报文后解析，把 selection/message/file cursor 转给 pi 的 LLM 工具链。
- **FloatFrame 浮窗框架**（`internal/action/floatframe.go`）：事件路由与 Display 顺序的集中管理，`SelectPane` 等浮窗通过它统一盖在 notePane 之上，确保鼠标/键盘事件被浮窗优先截获、绘制层叠正确。

**Changed**

- 主编辑器默认绑定 `Alt-Enter` 从原 micro 默认（`InsertNewline`）改为 `NotePaneOpen`。`internal/action/defaults_{darwin,other}.go` 各加一行。
- 协议名从早期 `EABP` 重命名为 `AIBP`（`Agent-IDE Bridge Protocol`），代码与文档全量同步；接收端目录从 `aibp-receivers/` 重命名为 `aibp-agents/`；注册表文件前缀从 `receiver-` 改为 `ai-`。
- `notePane.open()` 改造成 D16 参数化版本：`open(receiver)` 接收显式 receiver 入参，主编辑器打开前先 `aibp.Discover()`，≥2 个 receiver 时通过 SelectPane 让用户选。

**Fixed**

- notePane 关闭后 `BufPane.HandleEvent` 内访问 nil buffer 的 panic（D7 v2 修复）：v1 实现依赖 `close()` 时销毁 buffer，但 `open()` 重建 buffer 与 close 时序存在窗口期，会触发 nil 访问。v2 改成 `open()` 总是 `Close` 旧 buffer + `NewBufferFromString` 新 buffer，buffer 生命周期与 isOpen 状态机解耦。
- FloatFrame 关闭后终端光标残留在前序 pane 上：浮窗关闭前显式调用 `screen.ShowCursor(hideX, hideY)` 把光标藏到屏外，避免与主编辑器光标位置错位。
- 主编辑器选区在 notePane 发送后不清空，导致下一次发送时 selection 内容与已发送的不一致：发送成功后自动调 `Deselect` 清空主编辑器选区。

**Docs**

- `docs/agent-comm/` 整目录从 dev2 迁入，含 8 份"说明-*"当前态文档（架构设计 / AIBP / 发送端 / 接收端 / notepane）+ 8 份"Dx"决策文档（D11 名字分配、D12 多 receiver、D13 SelectPane、D14-D16 notePane 演化、D17 `:check-agent`）。README.md 提供"何时读哪份"导航矩阵。

## [1.0.12] - 2026-06-23

**Added**

- New `--reset-settings` CLI flag: copies the embedded `runtime/settings.json` (all 82 fields including MD extensions) to the user's config directory (`~/.config/microNeo/settings.json`), backing up any existing file to `settings.json.backup` first. Independent from `--clean`, which only does diff-only writes via `WriteSettings`.
- New docs site at https://sollawen.github.io/microNeo/ powered by mkdocs-material, bilingual EN/ZH, deployed via GitHub Actions. Includes `.github/workflows/deploy-docs.yml`, `mkdocs.yml`, and `docs/website/{en,zh}/index.md`.

**Changed**

- `runtime/settings.json` is now a complete reference file (JSON5) with inline comments for all 82 fields. Field values sourced from `internal/config/settings.go` + `internal/config/settings_md.go`; comments for the 74 native fields sourced from micro's `runtime/help/options.md`, plus hand-written descriptions for the 7 MD extension fields and `status-separator` (Powerline arrow U+E0B0). 11 fields are marked `[microNeo]` for git-overlay overrides. Layout reorganized into themed groups (theme / UI / Edit / Search / Brace / Files / Clipboard / Terminal / Plugin / Markdown) with English comments.
- `--options` flag removed; its listing role is subsumed by `--reset-settings` plus the newly-commented `runtime/settings.json` source.

**Fixed**

- Docs site no longer 404s at the root URL — `mkdocs-static-i18n` plugin now builds English content at `/` and Chinese at `/zh/`, with the 🌐 language switcher auto-generated in the header.

**Docs**

- New `docs/MKT/mkdocs-website-plan.md` capturing the mkdocs-material setup decisions (moved from dev2 working notes), including a new section 11 "Decision change history" covering the i18n plugin switch.

## [1.0.11] - 2026-06-22

**Fixed**

- Pasting a large block of text into an empty Markdown file no longer shows only the last `scrollmargin + 1` rows of the buffer. The cursor's vertical relocation in MD files was missing the `end-pin` branch (micro native Relocate branch 4): when the cursor lands near `bEnd` (short buffer / paste / goto-end), the view start was over-pushed toward the end and `coversExtent`'s buffer-end exception then locked the wrong state in place. `relocateVerticalMD` case C now branches on cursor position relative to `bEnd` — middle region keeps the old estimate, end region pins `bEnd` to the viewport bottom — restoring one-to-one alignment with micro's native 4-branch Relocate.

## [1.0.10] - 2026-06-19

**Fixed**

- Multi-cursor display regression in Markdown files: pressing `Shift-Alt-Up/Down` now shows all cursors again, instead of only the last one. Input/delete already worked correctly — only the on-screen rendering of secondary cursors was lost. Introduced in v1.0.6 (screenBuffer refactor).

## [1.0.9] - 2026-06-19

**Fixed**

- Typing at end of last line that triggers a softwrap no longer yanks the cursor to the top of the viewport.

## [1.0.8] - 2026-06-17

**Fixed**

- Pressing ESC in a Markdown file no longer collapses the whole screen back to raw markdown formatting.

## [1.0.7] - 2026-06-17

**Changed**

- MD diagnostic log now follows micro's debug switch, off by default in release builds.

**Fixed**

- Pressing Enter at the end of a buffer no longer causes incorrect screen scrolling.

## [1.0.6] - 2026-06-17

**Added**

- Export screen↔buffer coordinate conversion functions.

**Changed**

- Rewrite MD rendering pipeline to eliminate cursor drift when navigating across table and code-block segments.
- Improve table rendering and add display-package unit tests.

**Fixed**

- Cursor no longer drifts or disappears when navigating across table/code-block segments.
- Scrolling up no longer hides the cursor or leaves blank regions.
- Mouse scroll no longer gets stuck or falls back to raw formatting mid-scroll.
- Continuous ↓ across a table no longer makes the viewport jump.
- Clicking a table or code-block decoration row no longer jumps the cursor unexpectedly.

**Removed**

- `MDRender` and `MDRenderIdle` config options (rendering is now unconditional when MD is enabled).

## [1.0.5] - 2026-06-15

**Changed**

- Cursor vertical scrolling now works correctly in MD files, including across table and code-block segments.
- Tweak s-dark colorscheme status line colors.

**Fixed**

- End-of-file panic when softwrapping at the last line.

**Removed**

- Debug log from `initMDConfig`.

## [1.0.4] - 2026-06-11

**Added**

- Add screen offset reverse lookup function.

**Changed**

- Screen row → buffer line lookup now uses O(1) flat array.

**Fixed**

- Code-block borders now map to real buffer lines.

## [1.0.3] - 2026-06-09

**Changed**

- Tweak s-light colors.

**Fixed**

- Inline-code background color now renders correctly in all renderers.
