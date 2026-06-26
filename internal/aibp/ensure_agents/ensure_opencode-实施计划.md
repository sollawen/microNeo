# `ensure_opencode.go` 实施计划

> 位置：本文件与 `ensure_opencode.go` 同目录。
> 上游设计依据：`aibp-agents/opencode/ensure_opencode-修复计划.md`（D1/D2/D3 缺陷 + §4 修复方案代码）。
> 本计划是**落地 checklist**：改哪个文件、哪几行、为什么、怎么验。照着做即可。
> 状态：PLAN 模式，未经用户许可不改代码。

---

## 0. 背景

实测验证（2026-06-26，opencode 1.17.11 + aibp-opencode 1.0.2 npm）：`installOrUpdate` 在「pinned 版本已存在」场景下会**静默失败**（更新跑了却加载旧版）。根因是 D1+D2 两个缺陷叠加，连带 D3 误报。本计划修掉三者，并抽核心清理函数 `uninstallAIBP()`、暴露公开方法 `UninstallAIBP()`。

---

## 1. 改动范围

| 文件 | 改动类型 | 说明 |
|---|---|---|
| `ensure_opencode.go` | 改 | §2 全部代码改动 |
| `ensure_opencode_test.go` | **精简** | §3：删 `TestEnsure` + 修 pinned 用例 |
| `ensure.go` | **不改** | `AgentEnsurer` 接口无需动（`UninstallAIBP` 作为 `OpencodeEnsurer` 独享方法，不进接口，见 §2.4） |

---

## 2. 代码改动（`ensure_opencode.go`）

### 2.1 新增 `uninstallAIBP()` 核心清理函数（合并 D1 + D2 + 自检）

**核心重构**：抽出 `uninstallAIBP()` 内部函数承载所有「清理」逻辑，让 `installOrUpdate`（§2.2）和 `UninstallAIBP`（§2.4）共用。D1、D2、自检三者在语义上同属「确保清理彻底」，归于一处职责清晰、无重复。

**新增代码**：

```go
// uninstallAIBP 清理 aibp-opencode，保证「安装/卸载前是干净状态」：
//   1. 规范化 tui.json（删 aibp 条目）—— 修 D2
//   2. 自检（防 opencode 运行中持有锁，WriteFile 静默失败 → 旧条目仍在 → 后续 Already configured → 假成功）
//   3. glob 清所有版本 cache（@* 覆盖 @latest 与 pinned @1.0.x）—— 修 D1
// 被 UninstallAIBP（§2.4）和 installOrUpdate（§2.2）共用。
func uninstallAIBP() error {
	// 1. 规范化 tui.json（opencodeRemoveTuiEntries 对文件不存在/损坏返回 nil，不阻塞）
	if err := opencodeRemoveTuiEntries(); err != nil {
		return fmt.Errorf("规范化 tui.json 失败: %w", err)
	}
	// 2. 自检：tui.json 里不应还有 aibp-opencode 条目
	for _, e := range opencodeReadTui() {
		if strings.HasPrefix(e, aibpOpencodeSpec) {
			return fmt.Errorf("tui.json 规范化失败：条目仍存在 %q（请先完全退出 opencode TUI 再试）", e)
		}
	}
	// 3. glob @* 清所有版本 cache
	pkgGlob := filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@*")
	if matches, _ := filepath.Glob(pkgGlob); matches != nil {
		for _, m := range matches {
			_ = os.RemoveAll(m)
		}
	}
	return nil
}
```

**配套新增 `opencodeRemoveTuiEntries`**（照 `修复计划.md` §4.2 完整实现）：
- 用 `map[string]json.RawMessage` 读全文件，**只重写 `plugin` 键**，其它键（`$schema`/`keybinds`/`theme`）字节级保留 —— 保护用户配置的关键，不能简化成只写回 `Plugin` 字段。
- 删除条件：`e == aibpOpencodeSpec || strings.HasPrefix(e, aibpOpencodeSpec+"@")`，覆盖带/不带版本号两种写法。
- tui.json 不存在 / 损坏 → 返回 nil（不阻塞，`opencode plugin` 会新建）。
- **顺手加注释**：`WriteFile(p, ..., 0o644)` 会覆盖用户可能设的 `0600` 权限——tui.json 通常无 secrets，0644 是合理默认（写函数时就加，不另立步骤）。

> 注：`opencodeRemoveTuiEntries` 的错误**不静默吞**，由 §2.1 自检兜底。这与原 `修复计划.md` §4.1「非致命」注释冲突——以本计划为准（自检是更强的保障）。

---

### 2.2 `installOrUpdate` 改为复用 uninstall + 重装

**当前实现**（`ensure_opencode.go:120`）：

```go
func (e OpencodeEnsurer) installOrUpdate() error {
	cacheDir := filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@latest")
	_ = os.RemoveAll(cacheDir)                                  // ❌ D1：只删 @latest
	cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g")  // ❌ D2：不碰 tui.json
	if err := cmd.Run(); err != nil { return ... }
	return nil
}
```

**改法**：install 本质就是「清掉旧的 + 装新的」，清理部分正是 uninstall 的职责。复用后自身只剩两行核心：

```go
// installOrUpdate = uninstall + 重装（D1/D2/自检由 uninstallAIBP 集中解决）
func (e OpencodeEnsurer) installOrUpdate() error {
	if err := uninstallAIBP(); err != nil {
		return err
	}
	cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("opencode plugin 失败（可能 opencode 过旧，请升级 opencode）: %w", err)
	}
	return nil
}
```

---

### 2.3 新增 `opencodeNpmCacheSubdir()`（修 D3）

**位置**：`ensure_opencode.go` 约 85-105 行 `opencodeNpmAIBPVersion()` 当前硬编码 `aibp-opencode@latest`。pinned 安装后 `@latest` 不存在 → 误报「未安装」→ 触发 `InstallAIBP` → 掉进 D2。

**改法**：新增 `opencodeNpmCacheSubdir()` 按 tui.json 实际条目推导 cache 子目录，`opencodeNpmAIBPVersion` 用它替换硬编码路径。照 `修复计划.md` §4.3，**用带 `matched` 标志的版本**（Go 的 `break` 在 switch 内只跳出 switch，不能裸用）：

```go
// opencodeNpmCacheSubdir 按 tui.json 条目推导 cache 子目录名：
//   "aibp-opencode"        → "aibp-opencode@latest"
//   "aibp-opencode@1.0.2"  → "aibp-opencode@1.0.2"
// 找不到 aibp 条目 → 回退 @latest（兼容）。
func opencodeNpmCacheSubdir() string {
	suffix := "latest"
	for _, e := range opencodeReadTui() {
		matched := true
		switch {
		case strings.HasPrefix(e, aibpOpencodeSpec+"@"):
			suffix = strings.TrimPrefix(e, aibpOpencodeSpec+"@")
		case e == aibpOpencodeSpec:
			// suffix 默认即 "latest"
		default:
			matched = false // 非 aibp 条目（其它插件名 / 源码路径），继续看下一个
		}
		if matched {
			break // 命中第一个 aibp 条目即停（for 体内 break 跳出整个 for）
		}
	}
	return filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@"+suffix)
}
```

---

### 2.4 新增 `UninstallAIBP()` 公开方法（不进接口，为未来预留）

抽出 `uninstallAIBP()`（§2.1）后，暴露公开方法只需一行委托。当前 `AgentEnsurer` 接口（`ensure.go:31`）未含卸载、`Ensure()` 也不编排它，但：

- **未来加卸载功能零成本**：接口加签名 + 各 ensurer 实现，opencode 侧已备好。
- **opencode 比 pi 更需要**：pi 有 `pi uninstall` 命令可调，opencode 的 `plugin` 子命令无 remove/uninstall。
- **与 install/update 对称**：三者共用同一套底层，设计自洽。

**新增代码**（紧挨 §2.2 `installOrUpdate` 后面）：

```go
// UninstallAIBP 清理 aibp-opencode（规范化 tui.json + 删所有版本 cache）。
// 暂未被 Ensure 编排调用（AgentEnsurer 接口未含卸载），为未来 uninstall 功能预留；
// 逻辑与 installOrUpdate 共用 uninstallAIBP（§2.1）。
func (OpencodeEnsurer) UninstallAIBP() error {
	return uninstallAIBP()
}
```

**不进接口的决策**：进接口要改 `ensure.go` + `PiEnsurer`（也要实现一个），扩大改动范围。当前作为 `OpencodeEnsurer` 独享方法存在，未来需要时再统一提升接口——方法本身现在就是现成的。

---

## 3. 测试改动（`ensure_opencode_test.go`）

精简原则：**检测逻辑（`AIBPVersion`）的静默失败路径值得测；编排逻辑（`Ensure`）是 trivial 分支，mock 它价值低**。手动测（运行 opencode / 触发 ensure 看 infobar）能覆盖真实场景。

### 3.1 删 `TestEnsure` + `mockEnsurer`

**位置**：第 **246 行到文件尾**（约 110 行）。

删的内容：分隔注释块（246-255）、`mockEnsurer` 类型 + 5 个方法（256-280）、`TestEnsure` 含 6 个 branch（282-356）：source / not installed / outdated / update fails / ready / agent not found。

**为什么可删**：测的是 `ensure.go` 的通用编排（不是 opencode 特有逻辑，放这里位置本就不对）；全是 trivial `switch` 分支，几乎不可能写错；用 mock 对今天的 bug（D1/D2/D3 全在真实实现里）**零覆盖**。

### 3.2 修 `npm pinned version` 用例 cache 路径

**位置**：第 **75-98 行** `t.Run("npm pinned version", ...)`。现状是 D3 bug 的活体标本：tui.json 声明 `@1.0.1`，cache 却建 `@latest`，只有当前 buggy 代码才能通过。

**改法**：cache 路径 `@latest` → `@1.0.1`，与 tui.json 声明一致：

```go
// 改前（第 90 行，bug：tui.json 说 @1.0.1，cache 里却是 @latest）
pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@latest", "node_modules", "aibp-opencode")
// 改后（正确测试 pinned 行为）
pkgDir := filepath.Join(dir, "opencode", "packages", "aibp-opencode@1.0.1", "node_modules", "aibp-opencode")
```

`package.json` 内容（`aibp-1.5`）和断言 `(1,5,false)` **保持不变**。其它保留用例（`npm package` @40、`corrupt json` @130、`missing protocol` @161）都是 tui.json=`["aibp-opencode"]` + cache=`@latest`，D3 修复后仍通过，无需改。

### 3.3 不补 fallback 用例（精简原则）

不补「cache 与 tui.json 不匹配」的 fallback 用例——这类静默失败路径手动触发 ensure 看 infobar 也能发现，多一个 mock 用例 ROI 低。D3 回归保护靠 §5.2 手动流程 + 保留下来的检测用例。

---

## 4. 实施顺序

每步都能单独编译 + 单独验证：

1. **删 `TestEnsure` + `mockEnsurer`（§3.1）** → `go vet` / `go build` 通过，确认无残留依赖（`mockEnsurer` 只在此文件用）。
2. **修 pinned 用例 cache 路径（§3.2）** → 跑测试，此时该用例**会失败**（代码还没改 D3）。验证「测试确实在测 pinned 行为」。
3. **改 D3（§2.3）** → 跑测试，pinned 用例**应通过**，其它保留用例不受影响。
4. **新增 `uninstallAIBP()` + `opencodeRemoveTuiEntries`（§2.1，含顺手注释）**，再把 `installOrUpdate` 改为复用它（§2.2）→ `go build` 通过（靠手动验证 §5.2）。
5. **新增 `UninstallAIBP()` 方法（§2.4）** → `go build` 通过。
6. 全量 `go test ./internal/aibp/ensure_agents/...` 绿。

> D3 先于 D1/D2：D3 有现成测试可即时验证，D1/D2 靠手动验证，放后面集中验。

---

## 5. 验证清单

### 5.1 自动化（`go test`）

```bash
go test ./internal/aibp/ensure_agents/... -run Opencode -v
```

- [ ] `npm package`（@latest，非 pinned）✅ 通过
- [ ] `npm pinned version`（改成 `@1.0.1` cache）✅ 通过
- [ ] `npm package missing` / `corrupt json` / `missing protocol` ✅ 通过
- [ ] `empty plugin list` / `tui.json missing` / `tui.json corrupt` ✅ 通过
- [ ] 源码路径用例 ✅ 通过（不受影响）
- [ ] `TestEnsure` 已删 → `-run Ensure` 无匹配（预期）

### 5.2 手动（复现 pinned 现场 → 跑修复后的 `UpdateAIBP` 等价流程）

> ⚠️ 每步前**完全退出 opencode TUI**（§2.1 自检会拦，但提前退出最干净）。

```bash
# 0. 制造 D1/D2 现场：装 pinned 版
opencode plugin aibp-opencode@1.0.1 -g
jq '.plugin' ~/.config/opencode/tui.json        # ["aibp-opencode@1.0.1"]
ls ~/.cache/opencode/packages/ | grep aibp      # aibp-opencode@1.0.1

# 1. 跑修复后的 installOrUpdate（在 microNeo 里触发 UpdateAIBP，或临时写个 main 调）

# 2. 断言规范化
jq '.plugin' ~/.config/opencode/tui.json        # ✅ ["aibp-opencode"]（已规范化）
ls ~/.cache/opencode/packages/ | grep aibp      # ✅ 只剩 @latest（@1.0.1 已清，无泄漏）

# 3. 断言其它键未丢
jq 'keys' ~/.config/opencode/tui.json           # ✅ $schema/keybinds/... 仍在
jq -r '.keybinds.leader' ~/.config/opencode/tui.json  # ✅ ctrl+x（或用户原值）

# 4. 启动 opencode，左下角 ● 名字（确认加载最新版）
```

### 5.3 回归（源码版不受污染）

```bash
# tui.json 是路径形态时，installOrUpdate 不应误删路径条目
opencode plugin /path/to/microNeo/aibp-agents/opencode -g
# 跑修复后 installOrUpdate → jq 确认路径条目仍在
# （§2.1 的 opencodeRemoveTuiEntries 删除条件不匹配 "/" 开头路径）
```

---

## 6. 风险

| 风险 | 缓解 |
|---|---|
| `opencodeRemoveTuiEntries` 误删用户配置 | 用 `map[string]json.RawMessage` 只重写 plugin 键；§5.2/5.3 手动验证其它键保留 |
| opencode 运行中触发安装 → 假成功 | §2.1 自检拦截并给出「请先退出 opencode」提示 |
| glob `@*` 误删非 aibp 目录 | glob pattern 是 `aibp-opencode@*`，前缀精确，不会匹配其它插件 |
| 自检把「正常的空 plugin」误判为失败 | 自检条件是「条目 startswith aibp-opencode」，空数组不触发，安全 |
| 删 `TestEnsure` 失去编排逻辑覆盖 | 该逻辑是 trivial `switch` 分支，几乎不会写错；且 mock 对真实 bug 零覆盖（可接受取舍） |

---

## 7. 交付定义（DoD）

- [ ] §2.1（`uninstallAIBP` + `opencodeRemoveTuiEntries` + 注释）/ §2.2（`installOrUpdate` 复用）/ §2.3（D3）/ §2.4（`UninstallAIBP`）全部落地
- [ ] §3.1（删测试）/ §3.2（修 pinned）落地
- [ ] §5.1 全部测试绿
- [ ] §5.2 手动流程全过（pinned 现场 → 规范化 → 无泄漏 → 名字显示）
- [ ] §5.3 源码版回归通过
- [ ] `make build` 通过（项目要求，不能直接 `go build`）
- [ ] 同步更新 `修复计划.md`（把落地结果回填到 §2.5 / §6 验证记录）
