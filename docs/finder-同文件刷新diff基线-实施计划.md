# finder 同文件刷新 diff 基线 —— 实施计划

**性质**：实施计划。需求已明确（用户两轮描述 + 一轮澄清），机制已逐行验证，方案经 Lisa review 后从「重开 buffer」调整为「轻方案」（不重开，只重跑 onBufferOpen 钩子），本文给出精确改动清单与行为表，可直接照着改。

## 目标

让 finder 里对当前文件 Enter 时重跑 `onBufferOpen` 钩子，重新对齐 diff 基线，清掉陈旧 diff 标志。不重开 buffer，没有任何重开的副作用。

## 背景与需求

现状痛点（两个场景）：
- 未跟踪新文件（新建未 commit）：
  - 打开时无 diff 标志
  - 编辑后逐行出现标志
  - 改完后想一次清掉
- 已跟踪文件（刚 commit 过）：
  - 打开时无 diff 标志
  - 编辑后出现标志
  - 在外部 shell 里 commit 后，microNeo 内仍是旧标志

现有土办法：
- 关掉 fileA → 打开 fileB → 关掉 fileB → 再打开 fileA
- 本质是人为触发一次 `onBufferOpen`

期望交互：
- Ctrl-o（或 noName 的 Ctrl-q）打开 finder
- Enter 时光标停在 fileA 上（可以中途移走再移回来）
- 期望 pane 里 fileA 的 diff 标志全部消失，光标位置不动

## 机制根因

diff 标志的来源：
- diff 标志 = 当前内容对比「diff 基线」的差异
- 基线由 `runtime/plugins/diff/diff.lua` 的 `onBufferOpen` 钩子在打开 buffer 时快照

基线的两种取值：
- 已跟踪文件：`git show HEAD:./file` 成功 → 基线 = 当前 HEAD 内容
- 未跟踪文件：`git show` 失败 → 基线 = `buf:Bytes()`（打开瞬间的内容）

diff 插件的四个前置条件（任一不满足就不设基线）：
- `buf.Settings["diffgutter"]` 为真
- `not buf.Type.Scratch`
- `buf.Path ~= ""`
- `os.Stat(buf.AbsPath)` 成功（文件在磁盘上存在）

前置条件的推论：
- 从未落盘的新文件没有基线，重跑钩子也不会产生变化
- 非 git 仓库里 `git show` 必失败 → 基线恒为 `buf:Bytes()` → 一直无标志

陈旧标志的来源：
- 未跟踪新文件：基线 = 打开时的内容快照，之后所有编辑都算 diff
- 已跟踪文件：基线 = 打开时的旧 HEAD，外部新 commit 不触发任何刷新

重跑钩子为何能清：
- `SetDiffBase` 更新共享基线并重算共享 diff 图（buffer.go:1461）
- 已跟踪文件：重新取新 HEAD → 内容一致 → 无标志
- 未跟踪文件：重新取当前内容当基线 → 无标志

## 方案选择（为什么是轻方案）

两个候选：
- 重开方案：同路径 Enter 走 OpenCmd 重开 buffer，借 NewBuffer 重跑 onBufferOpen
- 轻方案：同路径 Enter 只重跑 onBufferOpen，不重开

轻方案胜出的维度：
- 不弹 Save 提示
- 光标与视图不动
- undo/redo 不动（saveundo 开启也不丢）
- 无 backup 残留
- 无重开的屏闪与重绘

轻方案与重开方案的共同点：
- 对齐基线的效果完全一致（都走 onBufferOpen 同一路径）
- 大文件（≥1000 行）都走 500ms 异步重算

已确认的取舍：
- 轻方案会顺带重跑其他插件的 onBufferOpen 钩子
- 全仓只有 3 个插件定义 onBufferOpen：
  - diff
  - ftoptions
  - literate
- ftoptions 幂等（按类型设 tabstospaces），结果不变
- literate 只对 .lit 文件重跑一次语法配置重载，普通文件直接 return

## 现状调研

同文件 no-op 的位置：
- `internal/action/fileops.go` 的 `onFinderClose`，Picked 分支
- 命中条件 `filepath.Join(r.Cwd, r.File) == h.Buf.AbsPath` 直接 return

轻方案的技术路径（已逐行验证）：
- `config.RunPluginFn("onBufferOpen", luar.New(ulua.L, h.Buf))` 与 buffer.go:536 打开 buffer 时触发的钩子同一条路径
- action 包已有先例：bufpane.go:319 用同样的调用方式跑 onBufPaneOpen
- `luar`（layeh.com/gopher-luar）与 `ulua`（internal/lua）在 action 包已 import
- diff.lua 收到 buffer 后走 SetDiffBase → UpdateDiff → 重算共享 diff 图
- diffBase / diff / Settings 都在 SharedBuffer 上（buffer.go:104/117/120），重算结果对共享同一 buffer 的所有 pane 生效

结论：
- 改动只有一处
- 不新增命令
- 不新增键位
- 不新增插件函数
- 不重开 buffer，内容与内存状态零扰动

## 实施计划

### 任务 1：同路径改为重跑钩子

改动文件：`internal/action/fileops.go`，`onFinderClose` 的 Picked 分支。

伪代码（任务 1）：
```
case finder.Picked:
    if filepath.Join(r.Cwd, r.File) == h.Buf.AbsPath {
        // 同文件：重跑 onBufferOpen 重对齐 diff 基线，清陈旧标志；不重开
        config.RunPluginFn("onBufferOpen", luar.New(ulua.L, h.Buf))
        return
    }
    h.OpenCmd([]string{filepath.Join(r.Cwd, r.File)})
```

具体动作：
- 同路径分支里把 `return` 改成先调 `config.RunPluginFn(...)` 再 `return`
- 新增 import：
  - `config`
  - `luar "layeh.com/gopher-luar"`
  - `ulua "github.com/micro-editor/micro/v2/internal/lua"`

### 任务 2：更新 onFinderClose 注释

改动文件：`internal/action/fileops.go`，`onFinderClose` 上方的 doc comment。

要点：
- Picked 语义从「同路径 no-op」改为「同路径刷新 diff 基线」
- 说明重跑 onBufferOpen、基线重新对齐的取值：
  - 已跟踪取 git HEAD
  - 未跟踪取当前内容
- 说明不重开 buffer，无 Save 弹窗、光标不动

### 任务 3：更新受影响的既有文档

改动文件：`docs/fileSelect/F9-add-实现方案.md`。

背景：
- 该文档 §7 边界表有一行引用「onFinderClose 的同文件 no-op」及旧行号 `fileops.go:50-52`
- 本计划把 no-op 改成刷新基线后，这行结论失效

动作：
- 更新该行，改为说明同路径会刷新 diff 基线而不重开

### 任务 4：验证

编译验证：
- `make build-quick`（跳过 generate，前提是 `runtime/syntax/*.hdr` 已存在）
- 若 hdr 缺失则先 `make generate` 或直接 `make build`
- 无编译报错

手动验证清单：

| 场景 | 操作 | 期望 |
|---|---|---|
| 未跟踪新文件 | 编辑出标志 → Ctrl-o → Enter | 标志清空，光标不动，无弹窗 |
| 已跟踪 + 外部 commit | 外部 commit → Ctrl-o → Enter | 标志清空，光标不动 |
| 已跟踪 + 未保存修改 | 编辑未保存 → Ctrl-o → Enter | 不弹 Save，未保存改动仍显示为 diff（基线 = HEAD，语义正确） |
| 取消 | Ctrl-o → Esc | 不变，回编辑态 |
| 不同文件 | Ctrl-o → 选 fileB → Enter | 关 fileA 开 fileB（原有行为不变） |
| 多 pane 同文件 | 分屏后重开 | 两个 pane 标志同步清空（共享 diff 图） |
| 大文件 | ≥1000 行文件 → Enter | 标志约 0.5s 后清空（异步重算） |
| .lit 文件 | 编辑 .lit → Enter | 顺带一次语法配置重载，可接受 |

## 风险与边界

轻方案的行为：
- 不弹 Save 提示（modified 未保存也直接对齐基线）
- 光标与视图不动
- undo/redo 不动
- 内容不从磁盘重读
- 大文件 diff 重算异步（约 0.5s）

顺带重跑其他插件钩子的影响：
- ftoptions 幂等，结果不变
- literate 仅 .lit 文件触发一次语法配置重载
- 语义耦合：同路径 Enter 触发的是「打开」钩子，将来若有插件在 onBufferOpen 里做非幂等的事需要留意

不进本计划的范围：
- 不改 finder 的 git 状态列
- 不改任何键位绑定
- 不新增命令
- 不改 diff.lua 本体

入口语义说明：
- browse（Ctrl-o）入口：guard 在「当前 buffer 就是选中文件」时命中，本改动对它生效
- quit（noName 的 Ctrl-q）入口：真正的 noName buffer AbsPath 为空，guard 本就不命中
- 「noName 出生、但 pane 里已打开过命名文件」时 guard 才会命中，本改动对它生效
- birth 入口（空 noName buffer）本就不命中 guard，本改动对它无影响
- file-born pane 的 Ctrl-q 仍是 Quit（QuitNeo 分流），不在本计划内

## 备选方案对比

备选 1：重开方案（OpenCmd 重开）：
- 同路径 Enter 走 OpenCmd，借 NewBuffer 重跑 onBufferOpen
- 优点：视觉上明确重开
- 缺点：
  - modified 时弹 Save 提示
  - 光标与视图重置
  - saveundo 开启且 mtime 未变时 undo 历史被 Unserialize 换掉
  - backup 文件残留
- 结论：不选，副作用多于轻方案

备选 2：diffreset 命令：
- 在 diff.lua 加 `config.MakeCommand`，把当前内容设为新基线
- 优点：
  - 不动光标
  - 不弹 Save
  - 不重开 buffer
- 缺点：新增命令与键位，且用户已明确要「finder Enter」的交互
- 结论：不选，仅记录

复用纪律说明：
- 已搜 `RunPluginFn` / `onBufferOpen`，重跑钩子走的是现成机制（buffer.go:536 同路径）
- 已搜 `luar` / `ulua` 在 action 包的使用先例（bufpane.go:319）
- 已搜全仓插件 onBufferOpen 定义，确认重跑的副作用面
