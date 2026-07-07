# FileSelector 架构设计

> **文档定位**：FileSelector 作为弹窗框架**第二个具体浮窗**的架构说明。
>
> **阅读对象**：需要理解弹窗框架扩展性、或要新增浮窗类型的开发者。架构与决策读本文档；交互细节与实现读代码。
>
> **关联**：[弹窗框架设计.md](../弹窗机制/弹窗框架设计.md)（框架本体）· [theme命令设计.md](../弹窗机制/theme命令设计.md)（第一个具体浮窗的使用方）

## 概述

`:file` 命令需要一个"浏览当前目录、导航进出、选中打开"的交互。这超出了 `SelectPane` 的能力边界（见 §分类原则），由新的具体浮窗 `FileSelector` 承载。

FileSelector 的架构意义**不在于它是个文件浏览器**，而在于：它是框架的第二个具体浮窗，且是**第一个具备两项新特性的浮窗**——

1. **会话内持久状态**：持有 `currentDir`，跨多次按键存活（SelectPane 开后即冻结）
2. **生命周期内重接容器**：导航时主动 `Close` + `Open` 重算几何（SelectPane 是一次性 open-close）

这两项特性是对弹窗框架的第一次真实压测。本文档记录压测结论：框架的"薄容器 + 4 函数契约"是否足够、何处出现需要显式化的隐含约定。

## 在框架中的位置

```
                 弹窗框架（弹窗框架设计.md）
                          │
        ┌─────────────────┴─────────────────┐
   FloatFrame（薄容器，所有浮窗共享）         │
   Open / Close / Display / HandleEvent      │
        │ 单向委托：display + handleEvent 函数值
        │
   ┌────┴─────────────┬───────────────────┐
   具体浮窗            │                   │
   SelectPane         FileSelector        （未来：命令面板 / mdOutline / …）
   - 无状态单选        - 会话状态 + 导航
   - 一次性            - 生命周期内重接容器
```

FileSelector 落在 `internal/action/`，与 SelectPane / FloatFrame 同包——可直接调 `TheFloatFrame`、`InfoBar`，无需新建包。与 SelectPane 平级、互不依赖、互不污染。

## 分类原则：何时建新浮窗，何时复用 SelectPane

这是 FileSelector 存在带来的**第一条可复用架构规则**，供后续所有浮窗决策（命令面板、mdOutline 等）套用：

> **SelectPane 服务"无状态单选"——N 个同质项里挑一个，挑完即关。一旦交互需要以下任一特性，建新具体浮窗：**
>
> | 特性 | SelectPane | 需新浮窗 |
> |------|-----------|---------|
> | 会话内持久状态（跨按键保留） | ❌ 开后冻结 | ✅ |
> | 项的异质处理（类型感知、条件行为） | ❌ 同质 | ✅ |
> | 生命周期超出"open→pick→close"（导航、多步） | ❌ 一次性 | ✅ |
> | 容器几何在生命周期内变化 | ❌ 固定 | ✅ |

FileSelector 命中全部四条。**这是它必须独立的根本理由，而非"代码量大了所以拆文件"。**

## 三个架构契约

### 契约 A（向上）：FileSelector 只懂路径，不懂 buffer

```
FileCmd ──Open(dir, …)──► FileSelector ──onOpen(path)──► FileCmd ──► 打开 buffer
```

FileSelector 对外的唯一耦合是 `onOpen(absPath string)` 回调——它报告"用户选了这个文件路径"，**不触碰 buffer / BufPane / 打开逻辑**。"在当前 bufWindow 打开 + 修改提示"全部留在 `FileCmd` 一侧。

**架构后果**：FileSelector 是**路径导航器**，不是":file 专用件"。未来 `:save-as`、`:cd`、`:vsplit <picker>` 都可复用同一个浮窗，只换 `onOpen`（或更通用的 `onPick`）回调。这条边界把"文件系统交互"与"buffer 操作"解耦，是本文档最重要的一条契约。

**多 pane / tab 语义**：micro 支持 split / tab 多 pane 同时编辑。FileSelector 的 UI 是全局单例（FloatFrame，受 C1 约束），但 `:file` 由**当前活跃 pane** 发起，`FileCmd` 把该 pane 捕获进 `onOpen` 闭包——打开的文件进**发起 `:file` 的那个 pane**，与 `:open` 行为一致。C3（模态）保证浮窗打开期间事件被锁、无法切 pane，目标 pane 不会中途易主。FileSelector 自身始终 pane-agnostic（契约 A），pane 绑定只存在于回调闭包内。

### 契约 B（向下）：重接容器，依赖框架约束 C4

FileSelector 导航时调 `TheFloatFrame.Close()` + `Open()` 重算几何。这引出一个**生命周期微妙点**：

> `FileSelector` 实例在 `Open` 之后、`Close` 之前的存活，**完全依赖 FloatFrame 持有的 `display`/`handleEvent` 函数值**（绑定到 `f`）。`Close` 会清空这两个引用——此刻若没有别的根引用 `f`，`f` 即可被 GC。

导航之所以安全，是因为 `navigateTo` **同步运行在 `f.handleEvent` 调用栈内**——调用栈本身是 `f` 的根引用，跨过 `Close` 存活到下一次 `Open` 重新注册。

**这把 FileSelector 绑死在框架约束 C4（同步用户驱动）上**：一旦框架未来引入异步驱动（goroutine 在 handleEvent 外触发导航），`f` 会在 `Close` 处被 GC，重开注册到一个已死对象。这是 FileSelector 对框架的**第一条隐含依赖**，应在本架构文档显式记录，而非埋在代码注释里。

### 契约 C（横向）：与 SelectPane 的 display 重叠

FileSelector 的列表渲染（视口、反色高亮、▲▼ 指示符）与 SelectPane 约 70% 重合。v1 **复制分化、不提取公共 helper**——沿用框架既有原则"任何下沉的能力必须证明所有浮窗都需要"。提取的触发条件见 §演化路径。

## 内部数据流：Model / View 分离

FileSelector 内部强制**加载与视图分离**——这是支撑 `.` 切换、排序、列显隐等所有交互的架构前提。

**Model（加载层）**——仅导航时重建，触碰 I/O：
- `entries []fileEntry`：当前目录的**全量**项，每项含完整 metadata（name / isDir / size / modTime / gitStatus）
- `loadDir(dir)` 一次把"所有文件 + 每个文件的全部信息"读全；session 内不可变，直到下次导航
- 代价：load 较慢（尤其大仓的 git-flag），但**每次导航只付一次**

**View（视图层）**——纯内存派生，零 I/O：
- 视图状态：`showHidden`、排序键、列显隐
- 可见列表 = `filter(entries, viewState)` → `sort(...)`，全部基于已加载 model
- `.` 切换 / 排序 / 列显隐只动 View，**绝不重读磁盘**

**为何必须分离**（而非"切换即重读"）：
1. 过滤/排序是用户视图偏好，与磁盘内容无关——目录不会因你按了 `.` 而变，重读是语义错误
2. git-flag 等昂贵 metadata 每次重算不可接受（大仓里反复 shell/读 `.git` 会卡）
3. 这把 ADR-2 的"改状态 + 重开"限定为**仅数据源变更（导航）**；视图变更走更轻的路径（见 ADR-2 修订）

**视图变更不触发重开**：`.` 隐藏文件后可见行数变，但 FloatFrame 几何**不变**——frame 在进入该目录时按其全量 model 算定，视图层只在固定 frame 内重绘可见子集（少则底部留白，多则滚动）。所以 `.` 只需 redraw，既不重读也不重开。

**重读与重开在 FileSelector 里同步发生，且只在导航时**：换目录既改变数据源（必须重读），也改变名字宽度（几何失效，需重开重算）。视图变更两者都不触发。于是本组件只有两类操作——**重操作（导航：重读 + 重开 + 重绘）**与**轻操作（视图：仅重绘）**。（概念上"重读≠重开"仍成立——别的浮窗可能基于缓存数据重算几何；只是 FileSelector 恰好让二者同步，因为唯一改变名字宽度的动作就是换目录。）

## 架构决策（ADR）

### ADR-1：独立具体浮窗，不扩展 SelectPane

**决策**：新建 FileSelector，SelectPane 维持"无状态单选"契约不变。

**理由**：FileSelector 命中分类原则全部四条特性（§分类原则）。把导航/状态/异质处理塞进 SelectPane，会让一个通用组件背上 file 专属能力，receiver / theme / 命令面板等所有 SelectPane 使用方都要拖着这些死代码。

**不可逆性**：高。这是组件边界定义，一旦合并极难拆回。

### ADR-2：导航刷新用"同实例重开"，不用"回调重建"

**决策**：导航在 `f` 内部改状态 + `Close`/`Open` 重接容器；不走"`onOpen` 回调里 `NewFileSelector()` 重建"。

**理由**：
- 回调重建要求状态跨实例传递（闭包链），栈链路分析复杂，每层 new 新对象
- 同实例重开是标准 OOP——组件拥有状态，主动让容器重算，方向正确（浮窗驱动容器）

**依赖**：框架约束 C4（见契约 B）。**这是本决策的硬前提**。

**适用边界（修订）**：此模式只服务于**导航**（`currentDir` 变）——既重读数据源，也重开重算几何。视图状态变更（`.` 切隐藏、排序、列显隐）走另一条路：从已加载 model 重新派生可见列表（§内部数据流），**不重读、不重开，仅重绘**（frame 对视图变更保持稳定）。两类不可混用。（上一版误以为视图变更也要重开，已纠正。）

**revisit 触发条件**：若框架放松 C4 引入异步驱动，本模式失效，需改为"容器持有浮窗强引用"或显式 session 对象。

### ADR-3：浮窗内几何变化用 `Open`/`Close` 重开，不给 FloatFrame 加 `Resize`

**决策**：FileSelector 借现有 `Open`/`Close` 实现导航时重算几何；不为本需求给 FloatFrame 新增 `Resize`/`Reflow` 方法。

**理由**：框架原则 C5（容器只做所有浮窗共享的事）。`SelectPane` 永不 resize，"生命周期内几何变化"目前只有 FileSelector 一个消费者——下沉到容器违反"必须证明所有浮窗都需要"。

**当前代价**：产生"close-to-resize"这一略显生硬的惯用法。它目前能用，是因为 C1（单浮窗）强制 `Open` 前必须 `Close`，重开是被 C1 **逼出来的**，而非为 resize 设计——属于"偶然可用"。

**触发频率已收敛**：v1 里重开只在**导航**时发生（换目录才改变名字宽度）；视图变更（`.`、排序）保持 frame 稳定、不重开。导航是低频重操作，在其中重开完全可接受，"偶然可用"的代价因此很小。

**revisit 触发条件**：当**第三个**浮窗也需要生命周期内几何变化时，"重开"惯用法重复出现 → 证明该能力确属公共需求 → 此时把 `Resize` 提升进 FloatFrame，并把 FileSelector 等迁过去。在第三个出现前，重开是可控的技术债。

### ADR-4：交互契约（产品决策，非架构）

以下为已确认的交互行为，记录为契约供实现与测试对齐，不展开架构论证：

| 行为 | 决策 |
|------|------|
| 导航键 | `←` 回父、`→` 进子、`↑↓` 移动（不循环） |
| 列表入口 | 不设 `../`，`←` 键负责回父 |
| `Enter` | 仅打开文件；目录上 `Enter` 为 no-op |
| 边界 no-op | `←` 在根目录、`→` 在文件上、`Enter` 在目录上 |
| `.` | 切换隐藏文件（dotfiles）显隐；默认显示 |
| 起始目录 | 当前 buffer 目录；无路径时 fallback cwd |
| 打开时当前已修改 | 弹保存提示（对齐 `:open`） |
| 排序 | 目录在前、文件在后，各自大小写不敏感 |
| title | 当前目录绝对路径（面包屑） |

## 演化路径

架构应能容纳已声明的未来需求而不破坏边界：

| 未来需求 | 演化方式 | 是否触动架构 |
|---------|---------|-------------|
| size / mtime / git 列 | model 已含全量 metadata（§内部数据流，加载时读全）；加列 = display 多列渲染 + contentSize 按列宽和算 | ❌ 不触动。纯视图层，无 I/O |
| 过滤/搜索、`h/l` 别名 | 基于已加载 model 派生可见列表（同 `.` 切换路径） | ❌ 不触动 |
| 命令面板 / mdOutline 等新浮窗 | 套用 §分类原则判断独立 vs 复用 | 取决于是否命中四特性 |
| 第三个列表型浮窗出现 | display 重叠第三次 → 提取 `drawList` 共享工具 | ✅ 触发契约 C 的提取 |
| 第三个浮窗需生命周期内几何变化 | → 触发 ADR-3 revisit，`Resize` 进容器 | ✅ 触发框架演化 |

**架构稳定性判据**：上述所有演化中，FloatFrame 的 4 函数契约（`Open/Close/Display/HandleEvent`）与向上契约（`onOpen(path)`）保持不变。这是本设计不制造未来债的保证。

## 对框架的反馈

FileSelector 作为第二个具体浮窗，对弹窗框架给出两点反馈：

**✅ 验证**：薄容器 + 4 函数契约**足以承载**会话状态 + 生命周期内重接容器 + 异质项处理三种新特性，**零容器改动**。"业务下沉到具体浮窗"的原则经受住了比 SelectPane 更复杂的用例。这给后续浮窗（命令面板、mdOutline）提供了信心：框架不需要为复杂度预先加料。

**⚠️ 暴露的开放问题**：`Open` 的"重入语义"目前是**隐含**的——`Open` 拒绝重开（C1）这一行为，被 FileSelector 当作"必须先 Close 再 Open"的 resize 机制来用。这能用，但属于"被 C1 逼出的偶然可用"，而非显式设计的 resize 契约。建议在框架文档（弹窗框架设计.md）补一条注记：*"Open 不支持热更新几何；浮窗需在生命周期内改几何时，用 Close+Open 重接（参见 FileSelector ADR-3）"*——把隐含用法显式化，避免后来者误以为 Open 可直接重入。

## 侵入与边界

| 原生 / 框架文件 | 是否修改 |
|----------------|---------|
| `internal/action/floatframe.go` | ❌ |
| `internal/action/selectpane.go` | ❌ |
| `cmd/micro/micro.go` | ❌（`InitNeoCommands()` 已在） |
| `internal/action/command_neo.go` | ✅ +1 行注册 |
| `internal/action/fileselector_neo.go` | ✅ 新增 |

**边界小结**：FileSelector 向上只暴露路径（契约 A），向下只依赖 C4（契约 B），向旁不污染 SelectPane（契约 C）。三向隔离是它能在未来被复用、且不制造框架债的根基。

## 开放问题（revisit 清单）

| # | 问题 | 触发 revisit 的条件 |
|---|------|-------------------|
| O1 | close-to-resize 惯用法是否升级为容器 `Resize` | 第三个浮窗需生命周期内几何变化（ADR-3） |
| O2 | display 逻辑是否提取共享 | 第三个列表型浮窗出现（契约 C） |
| O3 | FileSelector 是否独立成包 | 包内浮窗数量增长到 select/file/... 多类并存、出现共享内部类型时 |
| O4 | 异步驱动的需求 | 任一浮窗需 goroutine 触发状态变更（松弛 C4，冲击 ADR-2） |

---

## 附：实现入口（文件地图）

读代码时按此定位，**实现细节以代码为准**：

| 文件 | 看什么 |
|------|-------|
| `internal/action/fileselector_neo.go` | `FileSelector` 结构、`Open`/`navigateTo`/`display`/`handleEvent`、`readDirItems`、`FileCmd` |
| `internal/action/command_neo.go` | `InitNeoCommands` 注册 `:file` |
| `internal/action/selectpane.go` | 对照参考：无状态单选浮窗的形态 |
| `internal/action/floatframe.go` | `Open`/`Close` 契约、title 撑宽、sentinel、失败检查 |
