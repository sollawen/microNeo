## 基本规则

- **如非必要，勿增实体**。能复用已有代码和方法的，就不要新写代码的方法
- 编译本项目时，必须使用 `make build`，不要直接使用 `go build`。如果想快速编译跳过 generate 步骤，可以使用 `make build-quick`
- **首次 clone 或换机器后必须先跑一次 `make generate`**（或直接 `make build`）：`generate` 生成 `runtime/syntax/*.hdr`（filetype 检测头，被 `.gitignore` 忽略、不入库），只有 `*.yaml` 没 `.hdr` 时所有语法高亮会全失效（全文 default 色）。`make build-quick` 跳过 generate，已在仓库里跑过一次 generate 后才能安全使用
- 用户配置目录统一在 `$XDG_CONFIG_HOME/microNeo`（未设置时 fallback 到 `~/.config/microNeo`），启动时可用 `--config <dir>` 临时覆盖
- `docs/`目录是本项目所有设计方案和计划
- **代码注释不引用文档**：注释必须自包含，用散文讲清「为什么」，不依赖任何外部文档即可读懂。**禁止在代码注释里写设计文档的文件名或章节号**
- **特殊 Skill 触发**：当用户说 `commit` / `changelog` / `release` 时，**不要 summon 任何 assistant**。先读取 `.agents/skills/` 下对应的 `SKILL.md` 文件，按规则直接执行。这些是项目的特殊 skill，有专门的执行规则。

## Debug
- microNeo 有统一的 debug 日志机制，**不要自己另写一套**（自建日志文件、自建 print 等），一律复用下面这个
- 开关：对齐 micro 原生 `util.Debug`。`make build-dbg` 编译时注入 `util.Debug=ON`，运行时追加写 `/tmp/microNeo_debug.log`；`make build` / `make build-quick` 默认 `OFF`，不写日志、零开销
- 写日志（函数都在 `internal/display/bufwindow_md.go`，内部先判 `util.Debug=="ON"` 才写）：
  - `display` 包内：直接用 `dbgLog(format, args...)`
  - 其他包（如 `action`）：用导出的 `display.DbgLog(format, args...)`——`dbgLog` 的透传包装，共用同一个 sink
- 所有层（MD 渲染、分屏 resize 等）写进同一个文件，排查时用 `grep` 按各自前缀过滤

---


## 核心概念

- **Segment**：每行 buffer 属于一个 Segment。表格/代码块的 `Segment.Rows` 行数 **>** buffer 行数（frame 装饰行），是 v1.0.5 光标跨段滚动的核心难点。
- **检测/渲染分离**：`buffer 变化 → DetectSegments() → SharedBuffer.MDSegments → displayBufferMD() 读 → renderer 算 → 写屏`。detect 不依赖屏宽；render 按屏宽布局。触发点 `buffer.go:211-212`（NewBuffer）+ `1031-1032`（编辑增量）。
- **阅读/编辑模式**：`BufWindow.editMode`（`bufwindow.go:42`）。`observeEditModeToggle` 观察 ESC/click 切换；主循环里 `editMode && 光标在 seg 内` → 回退原生渲染。

