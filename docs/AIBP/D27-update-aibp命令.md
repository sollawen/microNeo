# D27 · `microneo --update-aibp`：主动拉取 aibp 扩展最新发布版

> **状态**：方案已定，待实施。
> **前置**：D25（`AgentEnsurer` 接口 + `Ensure` 编排）、D26（`--check-agent` CLI 入口 + `micro_neo.go` 聚合惯例）、D19/D22（aibp-opencode 安装机制）。

---

## 一、目标

新增 CLI flag `microneo --update-aibp`：对**没有自更新能力**的 aibp 接收端 agent（opencode、claude），主动把已装的 aibp 扩展升到 npm / marketplace 上的最新发布版。

与 `--check-agent` 分工：

| 命令 | 语义 | 是否联网 |
|---|---|---|
| `--check-agent` | 只读诊断：aibp 扩展在不在、协议兼容吗、要不要自愈 | 装/升时才联网 |
| `--update-aibp` | 主动「拉最新发布版」 | opencode 必联网（查 registry）|

---

## 二、现状问题（动机）

### 2.1 `--check-agent` 漏掉同协议 bugfix

`Ensure()` 编排**只在 protocol major 落后时**才触发 `UpdateAIBP()`。同协议的 bugfix 升级（如 aibp-opencode 1.0.4 → 1.0.5，协议都是 `aibp-2.0`）永远走 `default`（"ready"）分支，不更新。用户被卡在旧版，`--check-agent` 还报「ready」。

### 2.2 各 agent 自更新能力差异（关键）

| agent | 能否自检 / 提示新版 | `--update-aibp` 是否需要管 |
|---|---|---|
| pi | ✅ 有 in-app 升级提示，UX 足够好 | 否（跳过）|
| **opencode** | ❌ **完全不查**——`Npm.add` 见 cache 目录存在就直接用旧版，永久粘住 | **是（核心目标）** |
| claude | △ 无 in-app 提示，需手动 `claude plugin update` | 是（提供便利入口）|

**opencode 源码证据**（`packages/core/src/npm.ts` 的 `Npm.add`）：

```ts
const add = Effect.fn("Npm.add")(function* (pkg: string) {
  const dir = directory(pkg)
  const name = ...
  // cache 目录存在 → 直接返回旧版，不查 npm、不重新拉取
  if (yield* afs.existsSafe(path.join(dir, "node_modules", name))) {
    return resolveEntryPoint(name, path.join(dir, "node_modules", name))
  }
  // 只有 cache 不存在时才 reify（真正 npm fetch）
  const tree = yield* reify({ dir, add: [pkg] })
  ...
})
```

→ opencode 启动时对每个插件调 `Npm.add("aibp-opencode@latest")`，只要 cache 目录 `…/aibp-opencode@latest/node_modules/aibp-opencode` 存在就**永久用缓存的旧版本**，感知不到 npm 上已发布新版。cache 直到手动删除才会刷新。

**这正是 aibp-opencode 1.0.4 在 opencode 1.17.15 上卡住、1.0.5 发布后用户仍用旧版的根因**。唯一刷新路径是 uninstall + reinstall（删 cache）。opencode 自身永远不会做这件事，所以必须由 microNeo 主动补位。

---

## 三、方案

### 3.1 覆盖范围与三态分发

`--update-aibp` 只管 **opencode + claude**（pi 跳过）。每个 agent 按当前安装状态三态分发：

| 状态 | opencode | claude |
|---|---|---|
| 源码安装 | 删 tui.json 源码条目 + reinstall npm 最新 | N/A（marketplace-only）|
| 未安装 | reinstall npm（装最新）| install + enable |
| npm / marketplace 已装 | **查 npm registry**，新了才 reinstall，相同则跳过 | `claude plugin update`（幂等）|

**source → npm 转换**：只反注册源码路径（从配置里删条目），**不删磁盘上的源码文件**。打印 `source install → converting to npm latest`，绝不静默。

> footgun 提示：开发机上若在源码上调试时跑 `--update-aibp`，会把源码安装切到 npm。这符合「拉最新 npm 版」语义；目前仅作者一人用源码安装且调试结束也希望是 npm 最新，可接受。

### 3.2 接口扩展（`AgentEnsurer`）

加一个方法：

```go
type AgentEnsurer interface {
    // ... 现有方法保留；其中 AIBPVersion 签名 3→4（末位加 version，见 D27a），
    //     其余（AgentName / HasAgent / InstallAIBP / UpdateAIBP）不变 ...

    // UpdateToLatest 把该 agent 的 aibp 扩展升到最新发布版（三态分发，见 3.1）。
    // report 汇报进度 / 结果。各 agent 自行决定语义：
    //   - opencode/claude：真正干 source→npm / 装 / 查 registry 重装的活
    //   - pi：no-op + report "self-manages upgrades, skipping"（pi 有自己的 in-app 升级提示，
//     microNeo 不应插手；返回 nil 表示「无需 microNeo 更新，非错误」）
    UpdateToLatest(report Reporter) error
}
```

注：本方案同时把 `AIBPVersion()` 的签名从 `(int, int, bool)` 扩成 `(int, int, bool, string)`——末位多吐一个已装版本号 `version`（source / 未装时为 `""`）。这样 `UpdateToLatest` 一次调用就能同时拿到 `isSource` 与 `installed`，不必先调 `AIBPVersion()` 拿 `isSource`、再绕调内部 helper 把 `version` 重读一遍（反面论证见 3.3）。接口声明、`Ensure()` 调用点、pi / claude 两个实现的对齐细节见配套文档 **D27a**；D27 与 D27a 编译耦合，须一起落地。

三处实现：
- **pi**：`UpdateToLatest` 只 report 一句「self-manages, skipping」并返回 nil——「我不需要 microNeo 帮我更新」是「升到最新」的一个合法回答。
- **opencode**：见 3.3。
- **claude**：见 3.4。

**不加单独的 opt-out 标志**：pi 的「自管」策略内聚在它自己的 `UpdateToLatest` 里，`UpdateAll` 无需特判分支，保持纯统一循环。

### 3.3 opencode 的 `UpdateToLatest`（`ensure_opencode.go`）

**复用优先**：opencode「升到最新」的本质是 uninstall cache + 重装，现有 `installOrUpdate()` 已经做这件事（= `uninstallAIBP()` + `opencode plugin … -g`）。三态分发全部复用它，新逻辑只剩「查 registry 决定要不要装」一处判断。

```go
func (OpencodeEnsurer) UpdateToLatest(report Reporter) error {
    prefix := "aibp-opencode"
    _, _, isSource, installed := (OpencodeEnsurer{}).AIBPVersion() // 一次拿到 isSource + version

    // npm 已装 → 查 registry，新了才装；已是最新则跳过
    if !isSource && installed != "" {
        latest, err := latestNpmVersion(aibpOpencodeSpec)
        if err != nil {
            report(prefix + " couldn't check npm registry (" + err.Error() + "), skipping — retry later")
            return err
        }
        if !semverLess(installed, latest) {
            report(prefix + " already at latest (" + installed + ")")
            return nil
        }
        report(prefix + " updating " + installed + " → " + latest)
        return (OpencodeEnsurer{}).installOrUpdate() // 复用
    }

    // source / 未装 → 复用 installOrUpdate 拉最新 npm
    if isSource {
        report(prefix + " source install → converting to npm latest")
    } else {
        report(prefix + " not installed, installing latest")
    }
    return (OpencodeEnsurer{}).installOrUpdate() // 复用
}
```

`UpdateToLatest` 只调一次 `AIBPVersion()`，同时拿到 `isSource` 与 `installed`——前提是 `AIBPVersion()` 把 `version` 也吐出来（见下第 2、3 点）。否则就得像旧设计那样：调 `AIBPVersion()` 拿 `isSource`（它内部已读过 `package.json`、却把 `version` 丢掉），再单独调 `opencodeNpmAIBPVersion()` 把 `version` 重新读一遍。一次 CLI 无所谓性能，但「公开方法扔掉我需要的东西、再绕去调它的内部 helper 捡回来」是纯粹的逻辑拧巴，不接受。这正是把 `AIBPVersion` 签名扩到 4 值的动机（接口声明 + pi/claude 对齐见 **D27a**）。

对现有代码三处微调，**零新增 helper**：

1. **`opencodeRemoveTuiEntries()` 谓词加一行**，让它连源码路径条目（路径里含 `aibp-agents/`）一起删。于是 `installOrUpdate()`（经 `uninstallAIBP()` → `opencodeRemoveTuiEntries()`）自动完成 source→npm：先删源码条目、再重装 npm，无需另造 source 专用卸载函数。npm→npm 路径没有源码条目，新分支永不命中，行为不变：

   ```go
   // 改前（只删 npm 形态）：
   if e == aibpOpencodeSpec || strings.HasPrefix(e, aibpOpencodeSpec+"@") {
   // 改后（npm + 源码都删）：
   if e == aibpOpencodeSpec || strings.HasPrefix(e, aibpOpencodeSpec+"@") || strings.Contains(e, "aibp-agents/") {
   ```

   带 `/`（`aibp-agents/`）而非裸 `aibp-agents`：源码条目是路径，必含子目录（形如 `…/aibp-agents/opencode`）；裸子串会误伤名字里凑巧含 `aibp-agents` 的无关插件条目。

2. **`opencodeNpmAIBPVersion()` 顺带返回 `.version`**——它本来就读这个 `package.json`（现只取 `.aibp.protocol`）。签名 `(int, int, bool)` → `(int, int, bool, string)`，末位多吐一个 `version`。「读已装版本」零新增、零重复读文件。

3. **`AIBPVersion()` 方法也返回 `version`**——签名同样 3→4，把第 2 点 helper 吐出的 `version` 透传出来（source 分支 / 未装分支返回 `""`）。这是 `AgentEnsurer` 接口方法的签名变更，接口声明 + pi/claude 对齐属配套文档 **D27a** 的范围；opencode 自身这处透传随本步骤落地。

**不另造** `uninstallNpm` / `uninstallSource`：现有 `installOrUpdate()`（uninstall + reinstall）已覆盖两种卸载需求；source→npm 的「卸载」靠上面那一行谓词泛化融进既有路径。再造一对同名函数只会徒增理解成本与命名纠结。

### 3.4 claude 的 `UpdateToLatest`（`ensure_claude.go`）

```go
func (ClaudeEnsurer) UpdateToLatest(report Reporter) error {
    prefix := "aibp-claude"
    major, _, _, _ := (ClaudeEnsurer{}).AIBPVersion()

    // —— 未安装 → 装 ——
    if major == 0 {
        report(prefix + " not installed, installing ...")
        if err := (ClaudeEnsurer{}).InstallAIBP(); err != nil { // marketplace add + install + enable
            report(prefix + " install failed: " + err.Error())
            return err
        }
        report(prefix + " installed")
        return nil
    }

    // —— 已装 → 幂等 update（claude 自己查版本，最新就 no-op）——
    _, before, _ := claudeInstalledRecord() // claudeInstalledRecord 已返回 version
    report(prefix + " updating (claude plugin update) ...")
    if err := (ClaudeEnsurer{}).UpdateAIBP(); err != nil {
        report(prefix + " update failed: " + err.Error())
        return err
    }
    _, after, _ := claudeInstalledRecord()
    if before == after {
        report(prefix + " already at latest (" + after + ")")
    } else {
        report(prefix + " updated " + before + " → " + after)
    }
    return nil
}
```

claude 无 source 形态、无 registry 预查（marketplace 没有干净的「查最新版」HTTP 接口），靠 `claude plugin update` 幂等性达成「相同就算了」。

### 3.5 共享 helper（`ensure.go`）

```go
// latestNpmVersion 查 npm registry 上 pkg 的 latest 版本号。
// GET https://registry.npmjs.org/<pkg>/latest → JSON .version
// 仅 opencode 用（pi 跳过、claude 走 marketplace），放 ensure.go 作通用域工具。
func latestNpmVersion(pkg string) (string, error) {
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Get("https://registry.npmjs.org/" + pkg + "/latest")
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return "", fmt.Errorf("npm registry returned %d", resp.StatusCode)
    }
    var body struct{ Version string `json:"version"` }
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil { return "", err }
    if body.Version == "" { return "", fmt.Errorf("npm registry: empty version") }
    return body.Version, nil
}

// semverLess 报 a < b（仅支持 X.Y.Z 纯数字；aibp 包约定无 pre-release tag）。
func semverLess(a, b string) bool {
    pa := strings.Split(a, ".")
    pb := strings.Split(b, ".")
    for i := 0; i < len(pa) && i < len(pb); i++ {
        x, _ := strconv.Atoi(pa[i])
        y, _ := strconv.Atoi(pb[i])
        if x != y { return x < y }
    }
    return len(pa) < len(pb) // 1.0 < 1.0.0
}
```

**不引 semver 依赖**：aibp 包严格 X.Y.Z，10 行内联足够。若未来出现 pre-release tag 再考虑 `golang.org/x/mod/semver`。

### 3.6 批量编排（`ensure.go`）

```go
// UpdateAll 对所有已注册 agent 执行「升到最新发布版」编排：
// 跳过未装的；其余调 UpdateToLatest（pi 在自己的 UpdateToLatest 里 no-op 跳过）。
// 返回 hadError：是否有 agent 出错，调用方据此决定退出码。
func UpdateAll(report Reporter) (hadError bool) {
    if report == nil {
        report = func(string) {}
    }
    for _, e := range allEnsurers {
        if !e.HasAgent() {
            report(e.AgentName() + ": not installed, skipping")
            continue
        }
        if err := e.UpdateToLatest(report); err != nil {
            hadError = true
        }
    }
    return
}
```

### 3.7 CLI 入口（`cmd/micro/`）

**`micro.go`**：

```go
flagUpdateAIBP = flag.Bool("update-aibp", false,
    "Update aibp extensions for installed AI agents to the latest released version, then exit")
```

```
// flag.Usage 加一段
-update-aibp
    \tUpdate aibp extensions to the latest released version for agents that
    \tdon't self-update (opencode, claude). Checks the npm registry, reinstalls
    \tif newer. Prints progress to stdout and exits. Does not open the editor.
```

```go
// main()，紧跟 DoCheckAgent()（同样早退、零 config/screen 依赖）
DoCheckAgent()
DoUpdateAIBP()
```

**`micro_neo.go`**：

```go
// DoUpdateAIBP 执行 -update-aibp：把无自更新能力的 agent（opencode/claude）
// 的 aibp 扩展升到最新发布版，进度打到 stdout。跑完 exit，不进 TUI。
func DoUpdateAIBP() {
    if !*flagUpdateAIBP {
        return
    }
    hadErr := ensure_agents.UpdateAll(func(msg string) { fmt.Println(msg) })
    if hadErr {
        exit(1)
    }
    exit(0)
}
```

**与 `--check-agent` 的关系**：两者都早退；若同时传，`DoCheckAgent` 先跑先退（顺序见 main()）。无强互斥校验——YAGNI。

---

## 四、设计决策

| # | 决策 | 理由 |
|---|------|------|
| 1 | 只管 opencode + claude，pi 跳过 | pi 的 in-app 升级提示 UX 足够好；opencode 是唯一真正不自检的（源码证据），claude 无提示但 update 幂等 |
| 2 | opencode 用 npm registry 预查 + 条件 reinstall | opencode 永不自检、cache 永久粘住；registry 预查避免无谓 reinstall（几 KB 也省），「相同就算了」 |
| 3 | claude 不预查，直接 `claude plugin update` | marketplace 无干净「查最新版」HTTP；claude update 本身幂等（自检版本），效果等同 |
| 4 | registry 查不到 → 报错跳过，不盲 reinstall | 网络抖动不应触发意外 uninstall+reinstall；可预测性优先；hadError 提示用户重试 |
| 5 | source → npm 转换：反注册源码路径，不删源码文件 | 只切安装形态；源码是开发者的工作副本，不动 |
| 6 | 不加单独的 opt-out 标志，pi 在自己的 `UpdateToLatest` 里 no-op 跳过 | 「该不该管」只有 `UpdateAll` 一个消费者、只为 pi 一个 agent；策略内聚在 pi 自身，`UpdateAll` 保持纯统一循环，接口少一个方法 |
| 7 | `latestNpmVersion` / `semverLess` 放 ensure.go | 通用域工具（npm 语义），即使现仅 opencode 用；与 `EnsureAll` 同位 |
| 8 | 不引 semver 依赖 | aibp 包严格 X.Y.Z，10 行内联足够；YAGNI。**已知边界**：若 registry 的 `latest` 指向 pre-release（如 `1.1.0-alpha`），`semverLess` 会把它当正常版本比较，可能把已装的稳定版「升级」到 unstable。npm 惯例 `latest` 不指 pre-release，故当前不加 guard；一旦出现，在 `latestNpmVersion` 对含 `-` 的返回值告警 / 跳过，或切到 `golang.org/x/mod/semver` |
| 9 | flag 名 `--update-aibp`，与 `--check-agent` 并列 | 一个诊断、一个更新，职责清晰；命名延续 aibp 前缀 |

---

## 五、输出示例

```
$ microneo --update-aibp
pi: self-manages upgrades, skipping
opencode: 1.0.4 → 1.0.5, updating ...
opencode: updated
claude: already at latest (1.0.1)

$ echo $?
0
```

源码安装转 npm：

```
$ microneo --update-aibp
opencode: source install → converting to npm latest ...
opencode: updated to npm latest
```

registry 不可达：

```
$ microneo --update-aibp
opencode: couldn't check npm registry (Get "...": dial tcp: i/o timeout), skipping — retry later

$ echo $?
1
```

退出码：0 = 全部成功（含「已是最新」/「跳过」）；1 = 至少一个 agent 出错。

---

## 六、实施步骤

| # | 文件 | 改动 | 依赖 |
|---|------|------|------|
| 1 | `internal/aibp/ensure_agents/ensure.go` | `AgentEnsurer` 加 `UpdateToLatest()`；新增 `UpdateAll` + `latestNpmVersion` + `semverLess`；import `net/http` `encoding/json` `strconv` `time`。（`AIBPVersion` 接口声明 3→4、`Ensure()` 调用点加 `_` 属 D27a） | 无 |
| 2 | `ensure_pi.go` | 加 `UpdateToLatest`（report "self-manages, skipping" + return nil） | 1 |
| 3 | `ensure_opencode.go` | 加 `UpdateToLatest`（3.3，单次调 `AIBPVersion`）；`opencodeRemoveTuiEntries` 谓词加一行（含 `aibp-agents`）；`opencodeNpmAIBPVersion` 返回 `version`（3→4）；`AIBPVersion` 方法返回 `version`（3→4，透传 helper） | 1 |
| 4 | `ensure_claude.go` | 加 `UpdateToLatest`（3.4） | 1 |
| 5 | `cmd/micro/micro.go` | flag 声明 + usage + `main()` 调 `DoUpdateAIBP()` | 6 |
| 6 | `cmd/micro/micro_neo.go` | 新增 `DoUpdateAIBP()` | 1 |
| 7 | `CHANGELOG.md` | `[Unreleased]` 加 **Added** 条目（`--update-aibp` flag） | 1-6 |

改动量：~90 行净增（opencode/claude 的 `UpdateToLatest` + `latestNpmVersion` + `semverLess`），**零新增 helper、零删除**，3 处现有代码微调（谓词泛化 1 行 + `opencodeNpmAIBPVersion` 返回值扩展 + `AIBPVersion` 方法返回值扩展）。另有 `AIBPVersion` 接口声明 / `Ensure()` 调用点 / pi / claude 实现的对齐，见 **D27a**。每步后 `make build-quick`，全完成后 `make build`。

`AIBPVersion` 签名 3→4 会使现有 `_test.go`（ensure_pi/opencode/claude）里所有 `maj, min, isSource := …AIBPVersion()` 三值赋值编译失败——须逐处补第四个接收值（多数丢弃 `_`；版本相关断言改断 `version`）。opencode 测试归本步骤 3；pi/claude 测试归 D27a。接口断言 `var _ AgentEnsurer = …` 在主 `.go` 文件里，由 D27 + D27a 一起补齐方法 + 签名后成立。可选：给 `UpdateToLatest` 加表驱动测试（fake registry / mock exec）。

---

## 七、验收

| # | 场景 | 期望 |
|---|------|------|
| V1 | opencode npm 已装且是最新（1.0.5 == 1.0.5） | `already at latest (1.0.5)`；退出码 0；不 reinstall |
| V2 | opencode npm 旧版（1.0.4 < 1.0.5） | `1.0.4 → 1.0.5, updating ...` → `updated`；退出码 0；cache 被刷新 |
| V3 | opencode 源码安装 | `source install → converting to npm latest` → `updated`；tui.json 源码条目被删、npm 条目在；源码文件未删 |
| V4 | opencode 未安装 | `not installed, installing latest` → `installed` |
| V5 | opencode registry 不可达（断网） | `couldn't check npm registry (...), skipping`；退出码 1；不 reinstall |
| V6 | claude 已装最新 | `already at latest (1.0.x)`（before==after）；退出码 0 |
| V7 | claude 已装旧版 | `updated 1.0.0 → 1.0.1` |
| V8 | claude 未安装 | `not installed, installing` → `installed` |
| V9 | pi（已装） | `pi: self-manages upgrades, skipping`；退出码 0；**不动 pi 配置** |
| V10 | agent 未装（PATH 无 opencode） | `opencode: not installed, skipping`；继续下一个；退出码 0 |
| V11 | 不带 flag | 正常进 TUI |
| V12 | `make build` | 无编译错误、无未用 import；现有 `_test.go` 通过 |

---

## 八、与历史文档的关系

- **D25**（`AgentEnsurer` 接口）：接口扩展一个方法（`UpdateToLatest`）+ `AIBPVersion` 签名 3→4（末位加 `version`，pi/claude/接口声明的对齐见 D27a）；`Ensure` 编排逻辑不变（调用点多一个 `_` 丢弃 `version`）。opencode 侧微调 `opencodeRemoveTuiEntries` 谓词 + `opencodeNpmAIBPVersion` / `AIBPVersion` 返回 `version`，`installOrUpdate` / `uninstallAIBP` 复用不变。
- **D26**（`--check-agent` CLI）：`--update-aibp` 是对称新增的第二个 aibp CLI flag，复用 `micro_neo.go` 聚合惯例 + 早退 + `hadError` 退出码模式。
- **D19 / D22**（aibp-opencode 安装机制）：`installOrUpdate` / `uninstallAIBP` 复用；本方案补的是「何时触发 reinstall」的版本判断（opencode 自身永远不做）。
