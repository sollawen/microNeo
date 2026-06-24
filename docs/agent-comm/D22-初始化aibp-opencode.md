# D22 · :check-agent 初始化 aibp-opencode

> **状态**：已实现。

## 概述

`:check-agent` 命令遍历所有已注册的 agent（pi / opencode），对每个已安装的 agent 自动完成 aibp 扩展的检测与安装。用户只需在 microNeo 中执行一次 `:check-agent`，之后 Alt-Enter 就能向任意 agent 发消息。

## 架构

### 文件清单

| 文件 | 职责 |
|------|------|
| `internal/aibp/ensure_agents/ensure.go` | `AgentEnsurer` 接口 + `Ensure()` 编排 + `AllEnsurers` 注册表（D22 追加了 `AllEnsurers` slice） |
| `internal/aibp/ensure_agents/ensure_opencode.go` | opencode 的 `AgentEnsurer` 实现（本方案） |
| `internal/aibp/ensure_agents/ensure_opencode_test.go` | opencode 单测（9 用例全绿） |
| `internal/action/command_neo.go` | `CheckAibpCmd` — 遍历 `AllEnsurers` 调用 `Ensure()` |

### 遍历编排

```go
// ensure.go
var AllEnsurers = []AgentEnsurer{
    PiEnsurer{},
    OpencodeEnsurer{},
}

// command_neo.go
func (h *BufPane) CheckAibpCmd(args []string) {
    for _, e := range ensure_agents.AllEnsurers {
        if !e.HasAgent() {
            continue // 未装 → 静默跳过
        }
        _ = ensure_agents.Ensure(e, InfoBarNow)
    }
}
```

- `HasAgent() == false` 时静默跳过，不骚扰用户
- 单 agent 失败不影响其他 agent（串行 for 循环，错误已通过 Reporter 实时反馈）
- 未来加新 agent 只需在 `AllEnsurers` 追加一行，`command_neo.go` 不动

## OpencodeEnsurer 实现

### 五个方法

| 方法 | 实现 |
|------|------|
| `AgentName()` | `"opencode"` |
| `HasAgent()` | `exec.LookPath("opencode")` |
| `HasAIBP()` | 读 `tui.json` 的 `plugin[]`，匹配 `"aibp-opencode"` 或 `"aibp-opencode@x.y.z"` |
| `AIBPVersion()` | 读 `<cacheDir>/packages/aibp-opencode@latest/node_modules/aibp-opencode/package.json` 的 `aibp.protocol` 字段 |
| `InstallAIBP()` | 先 `rm -rf` 缓存目录，再执行 `opencode plugin aibp-opencode -g` |

### 路径解析

opencode 使用 `xdg-basedir` npm 包确定路径。microNeo 侧用 Go 实现相同逻辑：

**配置目录**（`opencodeConfigDir()`）：
```
XDG_CONFIG_HOME 设了 → $XDG_CONFIG_HOME/opencode
未设              → ~/.config/opencode
```

**缓存目录**（`opencodeCacheDir()`）：
```
XDG_CACHE_HOME 设了 → $XDG_CACHE_HOME/opencode
macOS fallback     → ~/.cache/opencode
Linux fallback     → ~/.cache/opencode
Windows fallback   → %LOCALAPPDATA%/opencode
```

> **关键**：不能用 Go 的 `os.UserCacheDir()`。它在 macOS 返回 `~/Library/Caches`，与 `xdg-basedir` 的 `~/.cache` 不一致。这是实施中发现的实际 bug——`AIBPVersion()` 读了不存在的 `~/Library/Caches/opencode/...` 导致 "version still invalid after reinstall"。

### 与 pi 的差异

| 维度 | pi | opencode |
|------|----|---------|
| 配置目录 | `$PI_CODING_AGENT_DIR` 或 `~/.pi/agent` | `~/.config/opencode` |
| 扩展登记表 | `settings.json` 的 `packages[]` | `tui.json` 的 `plugin[]` |
| 安装命令 | `pi install npm:aibp-pi` | `opencode plugin aibp-opencode -g` |
| 包位置 | `<agentDir>/npm/node_modules/aibp-pi/` | `<cacheDir>/packages/aibp-opencode@latest/node_modules/aibp-opencode/` |
| 缓存需手动清理 | 不需要 | **需要**（`rm -rf` 缓存目录，opencode 不会自动刷新 npm 缓存） |

## Ensure 编排流程（所有 agent 共用）

```
HasAgent → false  : 报错"not found"，返回 errAgentNotFound
          true    → 继续 ↓
HasAIBP →  false  : spawn InstallAIBP，报告 installed，继续 ↓
           true   : 继续 ↓
AIBPVersion → 成功 : 比对协议版本 ↓
              失败 : spawn InstallAIBP（自愈），再读一次 ↓
                       仍失败 : 报错"version still invalid"，返回
协议版本 :
  相等   : 报告 "ready"，返回 nil
  扩展<microNeo : 报告"协议过旧，请升级"（不自动升级）
  扩展>microNeo : 报告"microNeo 过旧，请升级"
```

## 单测覆盖

`ensure_opencode_test.go` 共 9 个用例：

| 测试 | 场景 |
|------|------|
| HasAIBP / unpinned | `"aibp-opencode"` → true |
| HasAIBP / pinned | `"aibp-opencode@1.0.1"` → true |
| HasAIBP / no entry | plugin[] 其他插件 → false |
| HasAIBP / tui.json missing | 文件不存在 → false |
| HasAIBP / tui.json corrupt | 非 JSON → false |
| AIBPVersion / normal | `aibp.protocol` 存在 → "aibp-1" |
| AIBPVersion / missing | package.json 不存在 → error |
| AIBPVersion / corrupt | 非 JSON → error |
| AIBPVersion / protocol missing | 无 `aibp.protocol` 字段 → error |

所有测试用 `XDG_CACHE_HOME` / `XDG_CONFIG_HOME` 环境变量隔离，不碰真实缓存和配置。

## 扩展新 agent

零侵入已有代码，只需三步：

1. 写 `ensure_<agent>.go`（4 方法 + 编译期断言）
2. 写 `ensure_<agent>_test.go`
3. 在 `ensure.go` 的 `AllEnsurers` 追加一行

## 相关文档

- **D17** (`docs/agent-comm/D17-初始化aibp-pi.md`)：`AgentEnsurer` 接口 + pi 实现的来源
- **D19** (`docs/agent-comm/D19-aibp-opencode.md`)：`aibp-opencode` TUI 插件的设计
- **D21** (`docs/agent-comm/D21-agent初始化的交互.md`)：`Ensure` 的 Reporter 模式 + InfoBarNow 实现
