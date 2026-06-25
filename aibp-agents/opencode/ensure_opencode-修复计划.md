# `ensure_opencode.go` 修复计划

> 目标文件：`internal/aibp/ensure_agents/ensure_opencode.go`
> 关联文档：`README.md`（已更新 opencode `plugin` 真实行为）。
> 本计划只描述代码改动方案，不直接改代码；执行需用户许可。

---

## 1. 现状（当前实现要点）

`installOrUpdate`（`InstallAIBP` / `UpdateAIBP` 共用）：

```go
func (e OpencodeEnsurer) installOrUpdate() error {
	cacheDir := filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@latest")
	_ = os.RemoveAll(cacheDir)                                  // 只删 @latest
	cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g")  // 无版本 spec
	if err := cmd.Run(); err != nil { return ... }
	return nil
}
```

- 清 cache：**只删 `aibp-opencode@latest`**。
- 重装前：**不动 tui.json**。
- `opencodeNpmAIBPVersion`：**只读 `@latest` 的 package.json**。

---

## 2. 实测结论（opencode `plugin` 真实行为）

已在本机实测验证（opencode 1.17.x）：

1. **按插件名去重，不是只追加**。tui.json 已有同名条目时重装打印 `Already configured`，**保留旧条目不变**，cache 会刷新。
2. **opencode 按 tui.json 条目加载对应 cache**：
   - 条目 `"aibp-opencode"` → 加载 cache `aibp-opencode@latest`
   - 条目 `"aibp-opencode@1.0.2"` → 加载 cache `aibp-opencode@1.0.2`
3. 无版本 spec 安装的规范形态：tui.json 写 `"aibp-opencode"`，cache 目录 `aibp-opencode@latest`。

---

## 3. 缺陷

### D1（cache 清理范围错误）—— 中危
`installOrUpdate` 只 `RemoveAll(@latest)`。若之前是 **pinned 版**手动安装（cache = `aibp-opencode@1.0.2`），删 `@latest` 是空操作，pinned cache 原地不动 → 重新加载的还是旧版本。

### D2（tui.json 未规范化）—— 高危，根因
`installOrUpdate` 重装前不改 tui.json。pinned 场景下：
- tui.json 仍是 `aibp-opencode@1.0.2`
- 跑 `opencode plugin aibp-opencode -g` → opencode 去重 → `Already configured` → tui.json 不变 → 仍加载 `@1.0.2`
- **更新静默无效**，且 opencode 会顺手新建 `@latest` cache 目录（**泄漏**）。

这是「`UpdateAIBP` 跑了但没生效」的真凶。

### D3（`AIBPVersion` 读错 cache）—— 中危，连带
`opencodeNpmAIBPVersion` 硬读 `@latest/package.json`。pinned 安装后 `@latest` 不存在 → 返回 `(0,0,false)` = **误报「未安装」** → 触发 `InstallAIBP` → 又掉进 D2。

---

## 4. 修复方案

### 4.1 `installOrUpdate`（核心）

三步，与 README「升级到新版本」流程对齐：

```go
func (e OpencodeEnsurer) installOrUpdate() error {
	// 1. 规范化 tui.json：删所有 npm 形态 aibp-opencode 条目
	//    （避免 Already configured 让重装静默失败）
	if err := opencodeRemoveTuiEntries(); err != nil {
		// 非致命：tui.json 损坏时跳过，后续 install 仍会写新条目
	}

	// 2. 清所有版本的 cache（glob @*，覆盖 pinned + latest）
	pkgGlob := filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@*")
	if matches, _ := filepath.Glob(pkgGlob); matches != nil {
		for _, m := range matches {
			_ = os.RemoveAll(m)
		}
	}

	// 3. 重装（无版本 spec → 规范 "aibp-opencode" 条目 + @latest cache）
	cmd := exec.Command("opencode", "plugin", aibpOpencodeSpec, "-g")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("opencode plugin 失败（可能 opencode 过旧，请升级 opencode）: %w", err)
	}
	return nil
}
```

要点：
- **glob 删 `@*`** 而非只删 `@latest`（修 D1）。
- **先规范化 tui.json**（修 D2）。
- 重装仍用无版本 spec，把任意旧形态（pinned / 残留）统一收敛到规范 `@latest`。

### 4.2 新增 `opencodeRemoveTuiEntries`（辅助）

⚠️ **关键陷阱**：tui.json 含其它键（`theme`/`keybinds`/…），不能只写回 `Plugin` 字段，否则会**丢用户配置**。必须保留所有其它键。

最安全的做法：用 `map[string]json.RawMessage` 读全文件，只重写 `"plugin"` 这一个键，其它键原样（RawMessage 字节级保留）写回：

```go
// opencodeRemoveTuiEntries 从 tui.json 删除 npm 形态的 aibp-opencode 条目
// （"aibp-opencode" 与 "aibp-opencode@<version>"）。
// 不动源码路径形态条目（源码↔npm 迁移由用户手动完成，见 README）。
// tui.json 不存在/损坏 → 静默返回，不阻塞 install。
func opencodeRemoveTuiEntries() error {
	p := opencodeTuiPath()
	b, err := os.ReadFile(p)
	if err != nil {
		return nil // 不存在：无条目可删
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil // 损坏：跳过，避免破坏
	}
	pluginRaw, ok := doc["plugin"]
	if !ok {
		return nil
	}
	var plugins []string
	if err := json.Unmarshal(pluginRaw, &plugins); err != nil {
		return nil
	}
	kept := make([]string, 0, len(plugins))
	removed := false
	for _, e := range plugins {
		if e == aibpOpencodeSpec || strings.HasPrefix(e, aibpOpencodeSpec+"@") {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return nil
	}
	out, _ := json.Marshal(kept)
	doc["plugin"] = out
	final, _ := json.MarshalIndent(doc, "", "  ")
	return os.WriteFile(p, final, 0o644)
}
```

> 选用 2 空格缩进（与 opencode 写 tui.json 的格式一致）。其它键字节级保留，只是整体重排缩进——可接受（opencode 重新读取不受影响）。

### 4.3 `opencodeNpmAIBPVersion` 硬化（优先级低，建议同修）

修完 4.1 后，microNeo 托管的安装永远是规范 `@latest`，D3 自动消失。但用户若手动 pinned，D3 仍误报。建议按 tui.json 实际条目推导 cache 目录：

```go
// opencodeNpmCacheSubdir 按 tui.json 条目推导 cache 子目录名：
//   "aibp-opencode"        → "aibp-opencode@latest"
//   "aibp-opencode@1.0.2"  → "aibp-opencode@1.0.2"
// 找不到条目 → 回退 @latest（兼容）。
func opencodeNpmCacheSubdir() string {
	suffix := "latest"
	for _, e := range opencodeReadTui() {
		switch {
		case strings.HasPrefix(e, aibpOpencodeSpec+"@"):
			suffix = strings.TrimPrefix(e, aibpOpencodeSpec+"@")
		case e == aibpOpencodeSpec:
			suffix = "latest"
		}
		break // 取第一个匹配
	}
	return filepath.Join(opencodeCacheDir(), "packages", aibpOpencodeSpec+"@"+suffix)
}
```

然后 `opencodeNpmAIBPVersion` 用 `opencodeNpmCacheSubdir()` 替换原硬编码 `aibp-opencode@latest` 路径即可。

---

## 5. 边界与错误处理

| 场景 | 行为 |
|---|---|
| opencode 未装 | `HasAgent()` 已挡住，`installOrUpdate` 不会到 |
| tui.json 不存在 | `opencodeRemoveTuiEntries` 静默返回；`opencode plugin` 会新建 |
| tui.json 损坏 | `opencodeRemoveTuiEntries` 静默返回（不破坏用户配置） |
| cache 目录为空（glob 无匹配） | `RemoveAll` 循环不执行，正常 |
| 用户装的是源码路径版 | tui.json 条目形如 `/path/.../opencode`，不匹配 `aibp-opencode` 前缀，**不动**（源码↔npm 迁移是用户手动行为；`AIBPVersion` 已把源码版识别为「已装」，不会触发 `installOrUpdate`） |

---

## 6. 验证清单

复现 pinned 场景 → 跑修复后的流程 → 断言规范化：

```bash
# 0. 前置：先手动装成 pinned 形态（制造 D1/D2 现场）
opencode plugin aibp-opencode@1.0.2 -g
jq '.plugin' ~/.config/opencode/tui.json          # ["aibp-opencode@1.0.2"]
ls ~/.cache/opencode/packages/ | grep aibp        # aibp-opencode@1.0.2

# 1. 跑修复后的 installOrUpdate 等价流程
#    （或在 microNeo 里触发 UpdateAIBP）

# 2. 断言
jq '.plugin' ~/.config/opencode/tui.json          # ✅ ["aibp-opencode"]（已规范化，无 @1.0.2）
ls ~/.cache/opencode/packages/ | grep aibp        # ✅ 只剩 aibp-opencode@latest（pinned cache 已清，无泄漏）

# 3. tui.json 其它键未丢
jq 'keys' ~/.config/opencode/tui.json             # ✅ theme/keybinds/... 仍在

# 4. 启动 opencode，左下角有 ● 名字（确认加载的是最新版）
```

回归：源码版场景（tui.json 是路径）跑 `installOrUpdate` 不应误删路径条目（见 §5）。

---

## 7. 不在本次范围

- 协议版本号 / 名字池逻辑（`aibp.ParseProtocol`、`aibp-names.json`）。
- 源码版 ensurer 逻辑。
- 其它 agent（pi 等）的 ensurer。
- opencode 升级（`plugin` 子命令无 update/remove，属 opencode 侧限制）。
