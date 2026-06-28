# `:theme` 命令设计

> 状态：设计中
> 关联：[弹窗框架设计.md](./弹窗框架设计.md) · [D13-SelectPane设计.md](../agent-comm/D13-SelectPane设计.md)

---

## 一、背景与动机

micro 原生命令 `set colorscheme <name>` 用于切换配色方案。命名沿用 Vim 的 `:colorscheme` 传统,但这个词对现代用户不友好:

- "theme" 是当下主流 IDE/编辑器(VS Code / Sublime / 各类 App)的通用词,"colorscheme" 已是终端老派词汇
- microNeo 目标用户(普通 markdown 阅读者)大概率只知道 "theme",看到 `colorscheme` 有认知门槛

microNeo 提供 **`:theme`** 命令,以现代交互方式(选择器浮窗)替换"必须记住主题名再输入"的老用法。

**设计原则**:

- microNeo 第一个自定义交互命令,以此**确立 neo 命令注册范式**
- 零侵入原生代码:通过 `action.MakeCommand` 动态注册,不碰 `commands` map、不碰 `SetCmd`
- 复用已有 `SelectPane` + `FloatFrame` 弹窗框架,不新建浮窗类型

---

## 二、交互流程

用户在命令栏输入 `:theme` 并回车:

1. 收集所有可用 colorscheme(内置 + 用户自写,共 9 个)
2. 在状态栏上方(`anchor.Y = -1`)弹出 `SelectPane` 浮窗,标题 `Themes`
3. 用户用 `↑/↓` 浏览,`Enter` 选中,`Esc` 取消
4. 选中 → 切换 colorscheme 并持久化;取消 → 关闭浮窗,无动作

```
┌─────────────┐  ← 紧贴 statusLine 上方
│ > default   │
│   dracula-tc│
│   gruvbox   │
│   ...       │
└─────────────┘
[statusLine]
```

---

## 三、技术实现

### 3.1 文件结构

| 文件 | 作用 | 性质 |
|------|------|------|
| `internal/action/command_neo.go` | microNeo 自定义命令的家(复活) | neo 通用增强 |
| `cmd/micro/micro.go` | 在 `InitCommands()` 之后调 `action.InitNeoCommands()` | 一行注入 |

**命名决策**:走 `*_neo.go`(通用增强)而非 `*_md.go`(纯 markdown)。因为 `:theme` 与 markdown 渲染无关,是编辑器通用功能的别名增强。

### 3.2 新增文件:`internal/action/command_neo.go`

```go
package action

import (
    "sort"

    "github.com/micro-editor/tcell/v2"
)

// InitNeoCommands 注册 microNeo 自定义命令。
// 在 cmd/micro/micro.go 的 action.InitCommands() 之后调用一次。
// 通过原生 MakeCommand 动态注册,不修改 commands map,零侵入。
func InitNeoCommands() {
    MakeCommand("theme", (*BufPane).ThemeCmd, nil)
}

// ThemeCmd 是 :theme 命令的 action。
// 不带参数 → 弹 SelectPane 让用户选;选中后切换 colorscheme 并持久化。
func (h *BufPane) ThemeCmd(args []string) {
    _, items := colorschemeComplete("") // input="" → 返回全部;复用原生 (见 3.4)
    sort.Strings(items)                // 对齐原生展示:colorschemeComplete 自身不保证顺序, OptionValueComplete 也做了 sort
    if len(items) == 0 {
        InfoBar.Error("no colorscheme found")
        return
    }

    // anchor.Y = -1 是 FloatFrame sentinel:紧贴 statusLine 上方 1 行
    anchor := Pos{X: 0, Y: -1}

    NewSelectPane().Open(items, "Themes", anchor, tcell.Style{}, func(picked *string) {
        if picked == nil {
            return // 用户按 Esc / resize,关闭即结束
        }
        // 选中 → 切换并持久化(writeToFile=true)
        err := SetGlobalOption("colorscheme", *picked, true)
        if err != nil {
            InfoBar.Error(err)
        } else {
            InfoBar.Message("theme: ", *picked)
        }
    })
}
```

### 3.3 注入点:`cmd/micro/micro.go`

`action.InitCommands()` 之后加一行:

```go
action.InitBindings()
action.InitCommands()
action.InitNeoCommands()   // ← microNeo 自定义命令
```

**为什么插在 `InitCommands` 之后**:`InitNeoCommands` 内部调 `MakeCommand`,依赖 `commands` map 已初始化(`InitCommands` 创建)。`InitPlugins`(在 `InitCommands` 之前的 `micro.go:318`)已加载插件 colorscheme,所以枚举时用户/插件主题都已就绪,无需额外等待。

### 3.4 colorscheme 列表获取:复用原生 colorschemeComplete

**不重复造轮子**。原生 `colorschemeComplete`(`infocomplete.go:63`)已实现了相同的枚举逻辑:

```go
// 原生实现(参考,不修改)
func colorschemeComplete(input string) (string, []string) {
    var suggestions []string
    files := config.ListRuntimeFiles(config.RTColorscheme)
    for _, f := range files {
        if strings.HasPrefix(f.Name(), input) {
            suggestions = append(suggestions, f.Name())
        }
    }
    // ...
    return chosen, suggestions
}
```

复用要点:
- `colorschemeComplete` 未导出,但 `command_neo.go` 与它同在 `package action`,可直接调
- 传 `input = ""` 时 `HasPrefix(name, "")` 恒真 → 返回全部名字(无过滤)
- 返回 `(string, []string)`,我们只需第二个 `[]string`
- **注意排序**:`colorschemeComplete` 自身不排序,但原生调用方 `OptionValueComplete`(`infocomplete.go:243`)在拿到结果后统一 `sort.Strings`。我们直接调该函数时,需自行 `sort.Strings` 才能对齐 `:set colorscheme <tab>` 的展示顺序

依据(底层 API,均不改):
- `RTColorscheme = 0`(`rtfiles.go:15`)
- `add(RTColorscheme, "colorschemes", "*.micro")`(`rtfiles.go:174`)已覆盖内置与用户两处目录
- `ListRuntimeFiles(RTColorscheme)`(`rtfiles.go:151`)返回 `[]RuntimeFile`,`Name()` 去后缀

**当前数量**:9 个(`default / dracula-tc / gruvbox / monokai / one-dark / s-dark / s-light / solarized / zenburn`)。

---

## 四、关键设计点与权衡

### 4.1 为什么直接复用 SelectPane

SelectPane 当前限制:**高度上限 10 行,不滚动**(`selectpane.go:60` 注释)。colorscheme 当前 9 个,正好 ≤ 10,无需改 SelectPane。

> **风险**:若未来 colorscheme 增至 >10 个,需要给 SelectPane 加滚动。
> 处置:届时作为独立任务,本设计不预设。当前 9 个的边界条件下,零改动复用是最优解。

### 4.2 锚点 `anchor.Y = -1` 的语义

`FloatFrame.Open` 对 `anchor.Y < 0` 有原生 sentinel 处理(`floatframe.go:108-110`):

> 负数 = 从 statusLine 上方第 |Y| 行起算

`Y = -1` → 浮窗左上角紧贴 statusLine 上方 1 行。这是 microNeo 用户期望的"主题选择器贴底弹出"位置,与 VS Code 命令面板的弹出位置观感一致。

### 4.3 持久化策略

调用 `SetGlobalOption("colorscheme", name, true)`(`command.go:673`) → `SetGlobalOptionNative`(634) → `doSetGlobalOptionNative`(575)。后者内部第 584 行检测到 `option == "colorscheme"` 会触发 `config.InitColorscheme()` + 全部 buffer `UpdateRules()`,切主题即时生效,无需额外代码。**与原生 `set colorscheme` 行为完全一致**,下次开机仍是该主题(`writeToFile=true` 写 `settings.json`)。

### 4.4 兼容性

- 老命令 `set colorscheme xxx` 原样保留,Vim 党不受影响
- `:theme`(不带参)弹选择器,不支持 `:theme <name>` 直接切换形式

### 4.5 命令名碰撞风险

`MakeCommand`(`command.go:75`)的实现是 `commands[name] = ...` 直接赋值,**无冲突检测**。若某插件先注册了同名 `theme` 命令,microNeo 的注册会**静默覆盖**它(或反过来被覆盖,取决于加载顺序)。

**当前处置**:不做检测。理由:
- microNeo 自定义命令目前仅 `theme` 一个,碰撞概率极低
- 加检测代码会让 `InitNeoCommands` 变重,与「第一个命令应极简」的定位不符
- 若未来 neo 命令增多,再统一加 `if _, exists := commands[name]; exists` 的守卫

**未来可选**(记录于此,本设计不实现):
```go
if _, exists := commands["theme"]; exists {
    InfoBar.Message("theme command already registered, skipped")
    return
}
```

### 4.6 复用 `InfoBar.Message` 而非自绘确认

切主题成功后用 `InfoBar.Message("theme: ", name)` 提示一行,与 micro 原生 `set` 命令的反馈风格对齐,不引入新的 UI 元素。

---

## 五、零侵入性核查表

| 原生文件 | 是否修改 | 说明 |
|----------|---------|------|
| `internal/action/command.go` | ❌ 不改 | `MakeCommand` 已是公开 API |
| `internal/action/command.go` `commands` map | ❌ 不改 | 动态注册 |
| `internal/action/selectpane.go` | ❌ 不改 | 直接复用 |
| `internal/action/floatframe.go` | ❌ 不改 | `Y=-1` 已支持 |
| `internal/config/colorscheme.go` | ❌ 不改 | 切换走原生 `SetGlobalOption` |
| `internal/config/rtfiles.go` | ❌ 不改 | `ListRuntimeFiles` 已提供 |
| `cmd/micro/micro.go` | ✅ 加 1 行 | `InitNeoCommands()` 调用 |

**结论**:除 `micro.go` 加一行注入外,所有改动落在 `command_neo.go` 新文件。完全符合 microNeo 低侵入原则。

---

## 六、回归测试点

实现后需验证:

1. **基础流程**:`:theme` → 浮窗弹出 → ↑/↓ → Enter → 主题切换 + statusLine 提示
2. **取消**:`:theme` → Esc → 浮窗关闭,主题不变
3. **持久化**:切换后重启 microNeo,主题保持
4. **列表正确性**:浮窗显示 9 个主题名(与 `ls runtime/colorschemes/` 一致)
5. **边界**:用户自写主题放入 `$XDG_CONFIG_HOME/microNeo/colorschemes/` 后出现在列表中
6. **兼容**:`set colorscheme gruvbox` 仍正常工作
7. **即时生效**:切换后已打开的 `.md` 文件渲染颜色立即变化(`InitColorscheme` + `UpdateRules`)
8. **不阻塞**:浮窗打开期间主编辑区不响应(modal),但 screen.Redraw 正常
9. **resize**:浮窗打开期间缩放终端,等同 Esc 取消(ADR-9),不崩
10. **已有浮窗时调用 `:theme`**:`FloatFrame` 拒绝重开(`selectpane.go` `Open` 返回 false) → 回调 `onSelect(nil)` → 主题不变、无提示。这是**已知行为**:回调的 nil 分支无法区分「用户 Esc」与「Open 被拒」,统一静默处理(Esc 时弹 message 反而打扰,与 VS Code 面板 Esc 行为一致)。若未来需区分,需在 `Open` 返回值上做文章,本设计不预设

---

## 七、未决 / 未来扩展

以下是有价值但**本设计暂不覆盖**的方向,记录于此供后续迭代参考:

- **预览**:当前浮窗只列名字。未来可在选中项右侧用对应主题色着色当前行,实现"悬停预览"(需扩展 SelectPane,当前不做)
- **删除内置主题的回收**:用户曾精简内置 colorscheme,若再删应同步更新本设计文档中的数量引用
