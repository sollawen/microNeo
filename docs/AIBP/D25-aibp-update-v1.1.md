# D25 — aibp 协议 x.x 比较 + UpdateAIBP 接口（任务4 实施计划）

> **类型**：实施文档（给执行者/agent 用）
>
> **状态**：🟡 草稿（2026-06-25，待用户拍板 §四 的设计选择后定稿）
>
> **来源**：todo.md 任务4；事实依据 `工作记录0625.md §四`。
>
> **权威文档**：协议版本语义以 `说明-AIBP.md §7` 为准，本文档只描述「从单版本号 → x.x 形式 + 自动升级」的本次实施。
>
> **前置依赖**：任务3（协议号已升 `aibp-2.0`）已完成；任务2（已切到源码路径）已完成。

---

## 一、目标

让 microNeo 的 `:check-agent` 命令具备两件新能力：

1. **协议版本 x.x 精确比较**：主版本**或**次版本任一不同 → 报告「outdated」，触发自动重装/升级
2. **协议一致后自动升级已装扩展**：调 `UpdateAIBP()` 把已装的 aibp 扩展拉到 latest（idempotent，失败不阻断）

---

## 二、范围与非目标

### 改（7 处代码）

| # | 文件 | 动作 |
|---|------|------|
| 1 | `internal/aibp/protocol.go` | **新建**：`ParseProtocol(s) (major, minor int, ok bool)` |
| 2 | `internal/aibp/registry.go` | `Discover()` 协议匹配改用 `ParseProtocol` |
| 3 | `internal/aibp/ensure_agents/ensure.go` | 接口加 `UpdateAIBP() error`；`Ensure()` 编排加「ready 后调 UpdateAIBP」 |
| 4 | `internal/aibp/ensure_agents/ensure_pi.go` | 实现 `UpdateAIBP()` |
| 5 | `internal/aibp/ensure_agents/ensure_opencode.go` | 实现 `UpdateAIBP()` |
| 6 | `internal/aibp/ensure_agents/ensure_pi_test.go` | 测试断言 `aibp-1` → `aibp-2.0` |
| 7 | `internal/aibp/ensure_agents/ensure_opencode_test.go` | 测试断言 `aibp-1` → `aibp-2.0` |

### 不改

| 项 | 理由 |
|----|------|
| 旧 `MajorVersion(s) int` 函数 | 保留（其它调用方可能依赖），不删 |
| aibp-agents/{pi,opencode}/ 源码 | 任务3 已完成 |
| `internal/aibp/message.go` 的 `Protocol` 常量 | 任务3 已改 |
| 文档（说明-AIBP.md / D17 / D19 等）的字符串示意 | 后续 D26 统一刷新（不在本次范围） |
| npm publish 操作 | 手动操作，README + 工作流说明 |

### 待用户拍板（§四 详述）

- **UpdateAIBP 怎么拿到 aibp 扩展源码目录**？——这是 §四 的核心设计选择

---

## 三、改动清单（精确）

### 改动 1：新建 `internal/aibp/protocol.go`

```go
package aibp

import (
	"strconv"
	"strings"
)

// ParseProtocol 解析 "aibp-2.0" 形式为主版本 + 次版本号。
// 任一段解析失败 → ok=false；返回的 major/minor 仅在 ok=true 时有意义。
//
// 与旧 MajorVersion 的区别：
//   - MajorVersion("aibp-1")    → 1
//   - ParseProtocol("aibp-1")   → (1, 0, true)   ← 旧形式默认 minor=0
//   - ParseProtocol("aibp-2.0") → (1, 1, true)
//   - ParseProtocol("aibp-2.0") → (2, 0, true)
//   - ParseProtocol("garbage")  → (0, 0, false)
//
// 形式约定（与说明-AIBP.md §7.1 一致）：
//   - 至少有一段主版本号在 '-' 之后
//   - 可选一段 '.' 分隔的次版本号
//   - 未来可能加 patch 段（aibp-2.0.3），本函数暂不解析 patch
func ParseProtocol(s string) (major, minor int, ok bool) {
	i := strings.LastIndexByte(s, '-')
	if i < 0 {
		return 0, 0, false
	}
	rest := s[i+1:]
	if j := strings.LastIndexByte(rest, '.'); j >= 0 {
		var err error
		major, err = strconv.Atoi(rest[:j])
		if err != nil {
			return 0, 0, false
		}
		minor, err = strconv.Atoi(rest[j+1:])
		if err != nil {
			return 0, 0, false
		}
		return major, minor, true
	}
	var err error
	major, err = strconv.Atoi(rest)
	if err != nil {
		return 0, 0, false
	}
	return major, 0, true
}
```

**测试**：`internal/aibp/protocol_test.go`（新建），覆盖：
- `"aibp-1"` → `(1, 0, true)`
- `"aibp-2.0"` → `(1, 1, true)`
- `"aibp-2.0"` → `(2, 0, true)`
- `"aibp-10.3"` → `(10, 3, true)`
- `""` → `(0, 0, false)`
- `"garbage"` → `(0, 0, false)`
- `"aibp-1.x"` → `(0, 0, false)`
- `"aibp-"` → `(0, 0, false)`
- `"aibp-2.0.3"` → 暂不解析 patch，返回 `(0, 0, false)`，或决定放行只取前两段（**待决**）

### 改动 2：`internal/aibp/registry.go` 的 `Discover()`

当前（line 59）：
```go
if MajorVersion(rf.Protocol) != MajorVersion(Protocol) {
    continue
}
```

改：
```go
extMaj, extMin, extOk := ParseProtocol(rf.Protocol)
myMaj, myMin, myOk := ParseProtocol(Protocol)
if !extOk || !myOk || extMaj != myMaj || extMin != myMin {
    continue
}
```

注释 `// "aibp-1"` / `// 字符串形如 "aibp-1"` / `// 解析 "aibp-1"` 三处不动（描述的是历史单版本形式，仍有参考价值）。

### 改动 3：`internal/aibp/ensure_agents/ensure.go`

#### 接口（line 30-37）加 `UpdateAIBP`：
```go
type AgentEnsurer interface {
	AgentName() string            // "pi" / "opencode"——日志和报错用
	HasAgent() bool              // 本机有没有这个 agent 程序
	HasAIBP() bool               // 该 agent 装没装 aibp 扩展
	AIBPVersion() (string, error) // 已装扩展实现的协议（如 "aibp-2.0"）。
	                              //   注意：协议版本，非包版本。读静态声明（package.json），不启动 agent
	InstallAIBP() error          // 装 aibp 扩展到该 agent
	UpdateAIBP() error           // 🆕 升级已装扩展到 latest（idempotent，失败不阻断 ensure 流程）
}
```

#### `Ensure()` 编排（line 79-88）改用 `ParseProtocol`，并在 ready 后调 UpdateAIBP：

```go
extMaj, extMin, extOk := ParseProtocol(ext)
myMaj, myMin, myOk := ParseProtocol(aibp.Protocol)
if !extOk || !myOk {
    report(prefix + " invalid protocol string, treating as outdated: " + ext)
    return errExtensionOutdated
}
switch {
case extMaj == myMaj && extMin == myMin:
    report(prefix + " ready")
    // 🆕 升级到 latest（idempotent；失败 best-effort，不阻断）
    report(prefix + " updating to latest ...")
    if uerr := e.UpdateAIBP(); uerr != nil {
        report(prefix + " update failed (using installed version): " + uerr.Error())
    } else {
        report(prefix + " updated to latest")
    }
    return nil
case extMaj < myMaj || (extMaj == myMaj && extMin < myMin):
    report(e.AgentName() + " protocol outdated, please upgrade")
    return errExtensionOutdated
default:
    report("microNeo protocol outdated, please upgrade")
    return errMicroNeoOutdated
}
```

#### `errExtensionOutdated` 错误消息更新（line 18）：

旧：
```
errExtensionOutdated = errors.New("aibp 扩展协议版本过旧，请运行 `pi update npm:aibp-pi` 后重启 pi")
```

新：
```
errExtensionOutdated = errors.New("aibp 扩展协议版本过旧，请运行 :check-agent 自动升级，或手动 `pi update npm:aibp-pi` 后重启 pi")
```

（opencode 用户需要看不同的命令，但因为这是通用 err 文本、且消息只用于 InfoBar 提示，省略 opencode 的命令也不算大问题。**精确方案待 §四 定**。）

### 改动 4：`internal/aibp/ensure_agents/ensure_pi.go` — `UpdateAIBP` 实现

**核心问题**：UpdateAIBP 怎么知道 aibp-pi 源码在哪？

详见 §四 的设计选择。先给一个**占位实现**（最小可用版）：

```go
func (PiEnsurer) UpdateAIBP() error {
    // 占位：当前用环境变量 MICRONEO_ROOT 定位 microNeo 仓库根
    // TODO(§四)：正式方案确定后替换
    root := os.Getenv("MICRONEO_ROOT")
    if root == "" {
        return nil // 没有仓库信息 → no-op，不阻断 :check-agent
    }
    cmd := exec.Command("git", "pull")
    cmd.Dir = root
    return cmd.Run()
}
```

### 改动 5：`internal/aibp/ensure_agents/ensure_opencode.go` — `UpdateAIBP` 实现

同上模式：

```go
func (OpencodeEnsurer) UpdateAIBP() error {
    root := os.Getenv("MICRONEO_ROOT")
    if root == "" {
        return nil
    }
    cmd := exec.Command("git", "pull")
    cmd.Dir = root
    return cmd.Run()
}
```

### 改动 6 & 7：测试断言更新

`ensure_pi_test.go:113`：
```diff
- pkg := `{"aibp":{"protocol":"aibp-1"}}`
+ pkg := `{"aibp":{"protocol":"aibp-2.0"}}`
```
`ensure_pi_test.go:122-123`：
```diff
- if ver != "aibp-1" {
-     t.Errorf("AIBPVersion() = %q, want %q", ver, "aibp-1")
+ if ver != "aibp-2.0" {
+     t.Errorf("AIBPVersion() = %q, want %q", ver, "aibp-2.0")
```

`ensure_opencode_test.go:126`：
```diff
- if err := os.WriteFile(pkgPath, []byte(`{"aibp":{"protocol":"aibp-1"}}`), 0644); err != nil {
+ if err := os.WriteFile(pkgPath, []byte(`{"aibp":{"protocol":"aibp-2.0"}}`), 0644); err != nil {
```
`ensure_opencode_test.go:134-135`：
```diff
- if ver != "aibp-1" {
-     t.Errorf("AIBPVersion() = %q, want %q", ver, "aibp-1")
+ if ver != "aibp-2.0" {
+     t.Errorf("AIBPVersion() = %q, want %q", ver, "aibp-2.0")
```

---

## 四、关键设计选择（待用户拍板）⭐

**问题**：UpdateAIBP 怎么知道 aibp 扩展的源码目录在哪？

**候选方案**：

### 方案 A：环境变量 `MICRONEO_ROOT`（占位方案，最简单）

- `os.Getenv("MICRONEO_ROOT")` 拿 microNeo 仓库根
- 在仓库根执行 `git pull` 拉 aibp-agents/ 子目录
- 缺点：用户必须手动设置环境变量；生产期对 microNeo 二进制无用
- 优点：实现最简单，不改接口签名

### 方案 B：Ensure 接收 sourcePath 参数

- `Ensure(e AgentEnsurer, sourcePath string, report Reporter) error`
- 调用方（`action/command_neo.go`）传 microNeo 仓库根
- 接口签名变：`InstallAIBP(sourcePath)`、`UpdateAIBP(sourcePath)`
- 缺点：要改 `command_neo.go` 的调用方；要改所有 `Ensure` 调用
- 优点：显式、可控、不依赖环境变量

### 方案 C：从 settings.json 反查

- `UpdateAIBP()` 自己读 `~/.pi/agent/settings.json` / `~/.config/opencode/tui.json`
- 找到 aibp-pi / aibp-opencode 对应的路径条目
- 拿到路径后 `git pull` 该路径或其父目录
- 缺点：每个 UpdateAIBP 实现都要解析自己的 settings；耦合
- 优点：不需要外部传参；自包含

### 方案 D：混合 — 优先 settings.json，fallback 环境变量

- 先按方案 C 反查
- 反查失败（settings 里没路径条目，只有 npm spec）→ fallback 到 `MICRONEO_ROOT`
- 缺点：实现复杂度最高
- 优点：覆盖所有场景（源码路径安装 + npm 安装 + 开发期）

**我的倾向**：**方案 C**（自包含、不依赖外部状态）。但需要确认 `~/.pi/agent/settings.json` 的解析逻辑当前在 `ensure_pi.go` 里**没有**，要新写。

请选择 A / B / C / D 之一，或提出新方案。

---

## 五、执行顺序

按依赖关系排：

1. **改动 1**（新建 protocol.go + 测试）—— 独立，可先做
2. **改动 2**（registry.go 用 ParseProtocol）—— 依赖 1
3. **改动 6/7**（测试断言更新到 aibp-2.0）—— 独立
4. **改动 3**（ensure.go 接口 + 编排）—— 依赖 1
5. **改动 4/5**（UpdateAIBP 实现）—— 依赖 §四 方案确定
6. **`make build-quick` + `go test ./internal/aibp/...`** —— 全部完成后验证

---

## 六、风险与决策

| 风险 | 应对 |
|------|------|
| UpdateAIBP 设计未拍板 | §四 阻塞整体进度，必须先定 |
| `ParseProtocol` 对未来 `aibp-2.0.3` patch 段的处理 | §三 改动 1 测试中标注「待决」，可默认「不支持 patch 段」或「支持只取前两段」 |
| UpdateAIBP 失败不阻断 ensure，可能掩盖问题 | report 里明确写「update failed (using installed version)」让用户看到 |
| `MajorVersion` 保留但没人用 | 留着不删，明确标记 deprecated；下个 D 文档再清 |

---

## 七、npm publish（不在本次代码改动范围）

`todo.md 任务4` 最后一条「npm publish pi and opencode aibp version 1.1」是手动操作，由用户执行。命令：

```bash
# pi 端
cd /Users/sollawen/pi-dev/microNeo/aibp-agents/pi
npm version patch  # 1.0.1 → 1.0.2（不是 1.1.0，npm 版本号和协议号是两套）
npm publish

# opencode 端
cd /Users/sollawen/pi-dev/microNeo/aibp-agents/opencode
npm version patch
npm publish
```

> ⚠️ **npm 版本号 vs aibp 协议号是两套**：
> - aibp 协议号（`aibp-2.0`）= 协议语义版本
> - npm 版本号（`1.0.1`）= 包发布版本，按 semver 自走
>
> 任务3 改的是 aibp 协议号，**不动 npm 版本号**。发版时 `npm version patch` bump npm 版本即可。

---

## 八、参考

| 项 | 位置 |
|----|------|
| UpdateAIBP 设计依据 | `工作记录0625.md §四` |
| 协议版本语义 | `说明-AIBP.md §7` |
| 决策背景 | `工作记录0624.md`（D23）、`工作记录0625.md`（D24） |
| 当前 `Ensure` 编排 | `internal/aibp/ensure_agents/ensure.go:47` |
| 当前 `MajorVersion` | `internal/aibp/registry.go:84` |
| 当前 `InstallAIBP` 实现 | `internal/aibp/ensure_agents/ensure_pi.go:96`、`ensure_opencode.go:121` |