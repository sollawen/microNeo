# D25 — ensure 接口升级：getAIBPversion / install / update + 新编排

> **状态**：📝 实施计划（待执行），CTO 出品，2026-06-25
>
> **来源**：`todo.md` 任务4。任务1（update 命令调研，见 `工作记录0625.md`）、任务2/3（源码路径安装 + 改名 + 协议 2.0）均已完成，本计划在其结论上落地。
>
> **不是**：本文是实施计划，不含最终代码；执行者（worker）按本文逐条实现。
>
> **相关**：`工作记录0624.md`（D23 自动升级讨论）、`工作记录0625.md`（D24 update 命令调研）、`说明-AIBP.md §7`（协议版本语义）、`internal/aibp/ensure_agents/ensure.go`。

---

## 一、Goal

把 `:check-agent` 从「协议体检 + 首装/自愈」升级为「版本驱动 + 源码安装豁免 + 自动更新」：每个 agent 提供 `AIBPVersion()/InstallAIBP()/UpdateAIBP()` 三件套，`Ensure()` 按「源码→跳过 / 未装→装 / 过旧→升 / 过新→提示 / 一致→ready」编排。

---

## 二、现状与关键事实（执行前必读）

任务2/3 已把 dev 机器改成**源码路径加载**，但 `ensure_*.go` 还停在 npm 加载假设，**当前 `:check-agent` 实际已失效**：

| agent | 实际加载入口（实测） | 当前 ensure 代码假设 | 结论 |
|-------|------------------|------------------|------|
| **pi** | `~/.pi/agent/settings.json` 的 `packages: ["../../pi-dev/microNeo/aibp-agents/pi"]`（**源码相对路径**） | 找 `npm:aibp-pi`（`HasAIBP`）+ `npm/node_modules/aibp-pi/`（`AIBPVersion`） | ❌ 双双 miss，HasAIBP=false、AIBPVersion 报错 |
| **opencode** | `~/.config/opencode/tui.json` 的 `plugin: ["/abs/.../aibp-agents/opencode"]`（**源码绝对路径**） | 找裸名 `aibp-opencode` + cache 目录 | ❌ 不匹配，HasAIBP=false |

两个 aibp 包都声明 `aibp.protocol = "aibp-2.0"`（`aibp-agents/{pi,opencode}/package.json` 实测）。

### 2.1 pi 源码路径解析的权威依据（已查源码）

- pi 的 `package-manager.js:parseSource()`：非 `npm:/git:/github:/http(s):/ssh:` 前缀 → `type:"local"`。→ `"../../pi-dev/microNeo/aibp-agents/pi"` 是 local。
- local 包的安装路径：`getInstalledPath()` → `getBaseDirForScope(scope)` → **user scope 返回 `this.agentDir`**（`package-manager.js:1690`），再 `resolvePathFromBase(path, agentDir)`。
- ⇒ **pi 全局 settings 里的 local 条目，相对 `~/.pi/agent` 解析**（microNeo 现有 `piAgentDir()` 与之同源，可直接复用）。实测 `"../../pi-dev/microNeo/aibp-agents/pi"` 相对 `~/.pi/agent` → `~/pi-dev/microNeo/aibp-agents/pi` ✅。
- ⚠️ 注意：pi 的 loader `discoverAndLoadExtensions` 用 CWD 解析，但那是另一条加载链；**包身份判定走 package-manager，用 agentDir**。本计划以 package-manager 为准。

### 2.2 opencode 源码路径

tui.json 里是**绝对路径**，直接读即可。裸名（`aibp-opencode` / `aibp-opencode@x.y.z`）才走 `~/.cache/opencode/packages/aibp-opencode@latest/`。

### 2.3 协议版本格式变了

`Protocol = "aibp-2.0"`（带 minor）。现有 `MajorVersion()` 只认 `"aibp-X"`（`LastIndexByte('-')` 后 `Atoi("2.0")` 失败返回 -1）——**对 2.0 已坏**。需新增 `ParseProtocol`。`MajorVersion` 全仓只有 2 处真用（`registry.go:60`、`ensure.go:79`），可一并替换后删除。

### 2.4 install/update 命令（D24 调研结论，直接用）

| 方法 | pi 实现 | opencode 实现 |
|------|--------|--------------|
| `InstallAIBP` | `pi install npm:aibp-pi` | `rm -rf <cache>/packages/aibp-opencode@latest` + `opencode plugin aibp-opencode -g` |
| `UpdateAIBP` | `pi update npm:aibp-pi` | **同 InstallAIBP**（opencode 无原生 update，清 cache + 重装是唯一强制刷新路径） |

---

## 三、接口设计

### 3.1 `AgentEnsurer` 接口（`ensure.go`）

```go
type AgentEnsurer interface {
    AgentName() string

    HasAgent() bool   // 保留：CheckAibpCmd 用它跳过未装 agent

    // getAIBPversion：识别本 agent 已装的 aibp。
    // 返回 (major, minor, isSourceInstall)。
    //   - 源码路径安装 → isSourceInstall=true，major/minor=0（不读盘；编排会跳过；信任假设见 §七 风险 9）
    //   - npm/cache 安装 → 读到协议 → isSourceInstall=false
    //   - 未装 / 损坏 / 读不到协议 → (0, 0, false)（编排触发 InstallAIBP）
    // 约定：合法协议大号从 1 起；major==0 即「无法识别」。
    AIBPVersion() (major, minor int, isSourceInstall bool)

    InstallAIBP() error // 装最新 npm 包（首次 / 损坏自愈）
    UpdateAIBP() error  // 升到最新 npm 包（过旧时）
}
```

**变更点**：
- 删除 `HasAIBP() bool` —— 被 `AIBPVersion()` 取代（未装即 (0,0,false)）。
- `AIBPVersion()` 签名从 `(string, error)` 改为 `(int, int, bool)`（**复用同名**，语义升级；Go 习惯 PascalCase，对应需求里的 `getAIBPversion`）。
- 新增 `UpdateAIBP() error`。

### 3.2 `Ensure()` 编排（`ensure.go`）

⚠️ **对需求伪代码的一处修正**：需求把 `无法识别 → installAIBP` 放在版本比较**之后**。但 `无法识别=(0,0,false)`，而 microNeo 大号=2，会先命中 `0 < 2 → updateAIBP`，导致 install 分支永不执行。**正确顺序是把「无法识别」提到版本比较之前**：

```go
func Ensure(e AgentEnsurer, report Reporter) error {
    prefix := "aibp-" + e.AgentName()
    report("checking " + e.AgentName() + " ...")
    if !e.HasAgent() {                       // 兜底；CheckAibpCmd 已先过滤
        report(e.AgentName() + " not found")
        return errAgentNotFound
    }

    major, minor, isSource := e.AIBPVersion()
    mineMajor, _, _ := aibp.ParseProtocol(aibp.Protocol) // =2

    switch {
    case isSource:
        report(prefix + " source install, skipping")
        return nil
    case major == 0: // 无法识别 / 未装
        report(prefix + " not installed, installing ...")
        if err := e.InstallAIBP(); err != nil {
            report(prefix + " install failed: " + err.Error())
            return err
        }
        report(prefix + " installed")
        return nil
    case major < mineMajor:
        report(prefix + fmt.Sprintf(" outdated (aibp-%d < aibp-%d), updating ...", major, mineMajor))
        if err := e.UpdateAIBP(); err != nil {
            report(prefix + " update failed: " + err.Error())
            return err
        }
        report(prefix + " updated")
        return nil
    case major > mineMajor:
        report("microNeo protocol older than " + prefix + ", please upgrade microNeo")
        return errMicroNeoOutdated
    default: // major == mineMajor
        report(prefix + fmt.Sprintf(" ready (aibp-%d.%d)", major, minor))
        return nil
    }
}
```

- 删除 `errExtensionOutdated`（过旧不再报错，改为自动 update）。
- 保留 `errAgentNotFound`、`errMicroNeoOutdated`。
- minor 在 source 分支始终为 0（不读盘）；npm 分支返回实际值。minor **不参与分支**（同大号即兼容，符合 semver）。

---

## 四、实施步骤（逐条可执行）

### 步骤 1：aibp 包新增 `ParseProtocol`，删除 `MajorVersion`
**文件**：`internal/aibp/registry.go`

1. 新增（放在原 `MajorVersion` 位置）：

```go
   // ParseProtocol — 解析 "aibp-MAJOR" 或 "aibp-MAJOR.MINOR"。
   // 兼容旧 "aibp-X"（minor 缺省 0）。解析失败返回 (0,0,false)。
   func ParseProtocol(s string) (major, minor int, ok bool) {
       const prefix = "aibp-"
       if !strings.HasPrefix(s, prefix) { return 0, 0, false }
       rest := s[len(prefix):]
       parts := strings.Split(rest, ".")
       if len(parts) == 0 || parts[0] == "" { return 0, 0, false }
       maj, err := strconv.Atoi(parts[0])
       if err != nil { return 0, 0, false }
       min := 0
       if len(parts) >= 2 {
           if min, err = strconv.Atoi(parts[1]); err != nil { return 0, 0, false }
       }
       return maj, min, true
   }
```

2. `Discover()` 内第 60 行左右，将：
   ```go
   if MajorVersion(rf.Protocol) != MajorVersion(Protocol) {
       continue
   }
   ```
   改为：
   ```go
   pm, _, _ := ParseProtocol(rf.Protocol)
   mineMajor, _, _ := ParseProtocol(Protocol)
   if pm != mineMajor {
       continue
   }
   ```
3. 删除整个 `MajorVersion` 函数（含其 Deprecated 注释）。

### 步骤 2：改 `ensure.go` 接口 + 编排
**文件**：`internal/aibp/ensure_agents/ensure.go`
- 接口按 §3.1 改（删 `HasAIBP`，改 `AIBPVersion` 签名，加 `UpdateAIBP`）。
- `Ensure()` 按 §3.2 重写。
- 删除 `errExtensionOutdated`；`errMicroNeoOutdated` 文案保留。
- import 如需补 `fmt`。

### 步骤 3：改 `ensure_pi.go`
**文件**：`internal/aibp/ensure_agents/ensure_pi.go`

1. 删除旧 `HasAIBP()`、旧 `AIBPVersion() (string,error)`。
2. 新增内部 `piReadSetting() []string`（读 settings.json 的 `packages[]`）：

```go
   func piReadSetting() []string {
       b, err := os.ReadFile(piSettingsPath())
       if err != nil { return nil }
       var s struct{ Packages []string `json:"packages"` }
       if err := json.Unmarshal(b, &s); err != nil { return nil }
       return s.Packages
   }
```

3. 新增 `AIBPVersion() (int, int, bool)`：

```go
   func (PiEnsurer) AIBPVersion() (int, int, bool) {
       for _, entry := range piReadSetting() {
           // 识别规则：包含 "aibp-agents" → 源码路径；包含 "npm:aibp-pi" → npm 包
	       if strings.Contains(entry, "aibp-agents") {
	          	return 0, 0, true // 源码路径：不读盘
	       }
	       if strings.Contains(entry, "npm:aibp-pi") {
	          	return piNpmAIBPVersion() // npm 包：读版本号
	       }
       }
       return 0, 0, false // 没找到 aibp 条目
   }
   // 读 aibp-pi 的 npm 安装版本：package.json 的 aibp.protocol 交 aibp.ParseProtocol 解析
   func piNpmAIBPVersion() (int, int, bool) {
       pkgPath := filepath.Join(piAgentDir(), "npm", "node_modules", "aibp-pi", "package.json")
       b, err := os.ReadFile(pkgPath)
       if err != nil { return 0, 0, false }
       var pkg struct {
           AIBP struct{ Protocol string `json:"protocol"` } `json:"aibp"`
       }
       if err := json.Unmarshal(b, &pkg); err != nil { return 0, 0, false }
       if pkg.AIBP.Protocol == "" { return 0, 0, false }
       return aibp.ParseProtocol(pkg.AIBP.Protocol)
   }
```

4. `InstallAIBP()` 不变。
5. 新增 `UpdateAIBP()`：

```go
   func (PiEnsurer) UpdateAIBP() error {
       cmd := exec.Command("pi", "update", aibpPiSpec)
       if err := cmd.Run(); err != nil {
           return fmt.Errorf("pi update 失败: %w", err)
       }
       return nil
   }
```

### 步骤 4：改 `ensure_opencode.go`
**文件**：`internal/aibp/ensure_agents/ensure_opencode.go`

1. 删除旧 `HasAIBP()`、旧 `AIBPVersion() (string,error)`。
2. 新增内部 `opencodeReadTui() []string`（读 tui.json 的 `plugin[]`）：

```go
   func opencodeReadTui() []string {
       b, err := os.ReadFile(opencodeTuiPath())
       if err != nil { return nil }
       var s struct{ Plugin []string `json:"plugin"` }
       if err := json.Unmarshal(b, &s); err != nil { return nil }
       return s.Plugin
   }
```

3. 新增 `AIBPVersion() (int, int, bool)`：

```go
   func (OpencodeEnsurer) AIBPVersion() (int, int, bool) {
       for _, entry := range opencodeReadTui() {
           // 识别规则：包含 "aibp-agents" → 源码路径；包含 "aibp-opencode" → npm 包
	       if strings.Contains(entry, "aibp-agents") {
	          	return 0, 0, true // 源码路径：不读盘
	       }
	       if strings.Contains(entry, "aibp-opencode") {
	          	return opencodeNpmAIBPVersion() // npm 包：读版本号
	       }
       }
       return 0, 0, false // 没找到 aibp 条目
   }
   // 读 aibp-opencode 的 cache 安装版本：package.json 的 aibp.protocol 交 aibp.ParseProtocol 解析
   func opencodeNpmAIBPVersion() (int, int, bool) {
       pkgPath := filepath.Join(opencodeCacheDir(), "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode", "package.json")
       b, err := os.ReadFile(pkgPath)
       if err != nil { return 0, 0, false }
       var pkg struct {
           AIBP struct{ Protocol string `json:"protocol"` } `json:"aibp"`
       }
       if err := json.Unmarshal(b, &pkg); err != nil { return 0, 0, false }
       if pkg.AIBP.Protocol == "" { return 0, 0, false }
       return aibp.ParseProtocol(pkg.AIBP.Protocol)
   }
```

4. `InstallAIBP()` 不变。
5. 新增私有辅助 `installOrUpdate()` 并由 `InstallAIBP`/`UpdateAIBP` 调用：

```go
   func (e OpencodeEnsurer) installOrUpdate() error {
       cacheDir := filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@latest")
       _ = os.RemoveAll(cacheDir)
       cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g")
       if err := cmd.Run(); err != nil {
           return fmt.Errorf("opencode plugin 失败: %w", err)
       }
       return nil
   }
   func (OpencodeEnsurer) InstallAIBP() error { return OpencodeEnsurer{}.installOrUpdate() }
   func (OpencodeEnsurer) UpdateAIBP()  error { return OpencodeEnsurer{}.installOrUpdate() }
```

   注释说明 D24 结论：opencode 无原生 update，清 cache 重装是唯一强制刷新路径。

### 步骤 5：更新测试
**文件**：`ensure_pi_test.go`、`ensure_opencode_test.go`

- 删除/重写所有 `HasAIBP` 测试（接口已删）。
- `AIBPVersion` 测试改为断言 `(major, minor, isSource)`：
  - **source 用例（核心，验证不读盘）**：settings 放相对路径，**目标 dir 不放 package.json** → 期望 `(0,0,true)`（信任假设生效）。
  - npm 用例：settings 放 `npm:aibp-pi`，并在 `<agentDir>/npm/node_modules/aibp-pi/package.json` 写合法 protocol → 期望 `(2,0,false)`。
  - npm 损坏：放 `npm:aibp-pi`，但 `<agentDir>/npm/node_modules/aibp-pi/package.json` 不存在 → 期望 `(0,0,false)`。
  - npm 字段残缺：放 `npm:aibp-pi`，package.json 写 `{"name":"aibp-pi"}`（无 `aibp` 字段）→ 期望 `(0,0,false)`。
  - 空 packages：settings 写 `{"packages":[]}` → 期望 `(0,0,false)`。
  - opencode 同理：绝对路径用例→`(0,0,true)`；裸名+cache 用例→`(2,0,false)`；缺失→`(0,0,false)`。
- `aibp.ParseProtocol` 已在步骤 1 那边有单测（或放在 `registry_test.go`），这里不重复。
- 测试里 `t.Setenv("PI_CODING_AGENT_DIR", dir)` 仍用；opencode 用 `XDG_CONFIG_HOME`/`XDG_CACHE_HOME` 隔离。
- 可选：`ensure.go` 加一个 `TestEnsure` 编排测试（mock AgentEnsurer）覆盖 5 分支（source/未装/过旧/过新/一致）。

### 步骤 6：构建 + 验证
1. `make build-quick`（编译通过）。
2. `make test`（`go test ./internal/... ./cmd/...` 全绿）。
3. 手测：在 microNeo 里 `:check-agent`，期望 InfoBar 显示 `aibp-pi source install, skipping` 与 `aibp-opencode source install, skipping`（当前 dev 机是源码安装；source 分支不取版本号，所以无 `(aibp-X.Y)` 后缀）。
4. 回归 `:check-agent` 不报错、不卡死。

---

## 五、Files to Modify

| 文件 | 改动 |
|------|------|
| `internal/aibp/registry.go` | 加 `ParseProtocol`/`ProtocolMajor`；`Discover` 改用之；删 `MajorVersion` |
| `internal/aibp/ensure_agents/ensure.go` | 接口（§3.1）+ `Ensure` 编排（§3.2）；删 `errExtensionOutdated` |
| `internal/aibp/ensure_agents/ensure_pi.go` | 重写 `AIBPVersion` 为 (int,int,bool)：包含 "aibp-agents" → 源码，包含 "npm:aibp-pi" → npm；加 `piReadSetting`、`UpdateAIBP`；删 `HasAIBP` |
| `internal/aibp/ensure_agents/ensure_opencode.go` | 重写 `AIBPVersion` 为 (int,int,bool)：包含 "aibp-agents" → 源码，包含 "aibp-opencode" → npm；加 `opencodeReadTui`、`UpdateAIBP`；删 `HasAIBP` |
| `internal/aibp/ensure_agents/ensure_pi_test.go` | 重写为 (int,int,bool) 断言 + source 用例（验证不读盘） |
| `internal/aibp/ensure_agents/ensure_opencode_test.go` | 同上 |
| `internal/action/command_neo.go` | **无需改**（Ensure 签名不变，CheckAibpCmd 仍可用） |

## 六、New Files

**无。** 设计原则：**不共享文件 I/O 和 JSON 解析**（不同 agent 的 package.json 结构可能差异很大），只共享 `aibp.ParseProtocol` 协议字符串解析（步骤 1 新增，在 `aibp` 包）。

---

## 七、Risks & 开放问题

1. **编排顺序（已决）**：`无法识别(major==0)` 必须排在版本比较**之前**，否则 install 分支被 update 吞掉（见 §3.2 ⚠️）。已按正确顺序写定。
2. **pi local 路径解析基准**：源码路径分支**不解析成绝对路径**（信任假设），所以原来 `AIBPVersion` 里那段 `piResolveLocalPath` 删了。但是 `InstallAIBP`/`UpdateAIBP` 不受影响（它们走 `pi install` / `pi update` 命令，让 pi 自己解析）。若未来要给源码路径加 name 校验，需要重新拿回 `agentDir` 基准，理由见 §2.1。建议步骤 6 手测时 dev 机 `:check-agent` 仍能识别到 aibp-pi source。
3. **识别规则简化**：当前实现用简单的字符串匹配识别 aibp 条目：
   - **pi**：包含 `"aibp-agents"` → 源码路径；包含 `"npm:aibp-pi"` → npm 包
   - **opencode**：包含 `"aibp-agents"` → 源码路径；包含 `"aibp-opencode"` → npm 包
   - **接受此风险**：
     - 若用户安装了名为 `my-aibp-stuff` 的非 aibp 扩展，可能会被误判（但实践中 `"aibp-agents"` 路径和 `"aibp-opencode"` 包名都是项目约定的，冲突概率极低）
     - 源码路径分支不读盘验证，信任用户配置的正确性
4. **源码安装但路径失效**：源码路径分支**不读** package.json（信任假设），路径坏了也返回 `(0,0,true)` → 编排跳过，不触发 install/update。设计取舍：源码由用户自管，microNeo 不擅自覆盖；用户感知不到「路径坏了」也无所谓（他自己改的源码路径）。报告就是 `source install, skipping`，**不带路径**。
5. **opencode UpdateAIBP==InstallAIBP**：实现重复，已按 D24 结论共用一段私有 `installOrUpdate()` 复用（两个导出方法各自调它）。语义仍分（编排按场景调不同方法）。
6. **minor 不参与分支**：仅文案。ext 2.0 vs microNeo 2.1 视作 ready（同大号兼容）。如未来要 minor 级处理，再扩编排。
7. **联网副作用**：`update/install` 会联网几秒。`CheckAibpCmd` 在主 goroutine 同步阻塞执行（已有 `InfoBarNow` 机制刷新），延时可接受。源码安装豁免后，dev 机日常 `:check-agent` 不再联网。
8. **删除 `MajorVersion` 的影响面**：全仓仅 `registry.go`（改）+ `ensure.go`（改）真用，`ensure_pi.go:91` 是注释引用（顺手改掉）。无外部 import。

---

## 八、交付检查清单（执行者自检）

- [ ] `make build-quick` 通过
- [ ] `make test` 全绿，含 source-path 用例
- [ ] dev 机 `:check-agent` 显示两个 agent 均 `source install, skipping`，无报错
- [ ] `grep -rn MajorVersion internal/` 无残留（除注释说明）
- [ ] `grep -rn HasAIBP internal/` 无残留
- [ ] 接口 5 个方法（AgentName/HasAgent/AIBPVersion/InstallAIBP/UpdateAIBP）在 pi、opencode 两个实现里齐全
