# D20 — 注册表目录改名：`microneo-agent-bridge-<UID>` → `aibp-<UID>`

> **类型**：实施文档（给执行者/agent 用）。本文件本身是改名的权威依据。
>
> **来源**：用户决定缩短 AIBP 注册表目录名。命名与协议 `aibp-1`、文件前缀 `ai-*.json` 同源，更 coherent。
>
> **权威文档**：改名完成后，`说明-AIBP.md §3.1` 仍是注册表目录算法的权威定义；本文件只描述「从旧值到新值的一次性迁移」。

---

## 一、目标

把 AIBP 注册表目录名从

```
<base>/microneo-agent-bridge-<UID>
```

改为

```
<base>/aibp-<UID>
```

`<base>` 的回落算法（`$XDG_RUNTIME_DIR` → `$TMPDIR` → `/tmp`）与 `MNAB_REG_DIR` 调试覆盖**完全不变**，只改目录名这一段字面量。

## 二、范围与非目标

**改**：目录名字面量 `microneo-agent-bridge` → `aibp`（共 3 处代码 + 9 处文档，清单见 §三）。

**不改（明确非目标）**：

| 项 | 是否改 | 理由 |
|----|--------|------|
| `MNAB_REG_DIR` 环境变量名 | ❌ 不改 | 它是「调试覆盖」旋钮，与「目录名」正交；改名会多动 3 端 + 全部文档，YAGNI |
| base 回落算法 | ❌ 不改 | 仅目录名这一段变化 |
| `ai-*.json` / `ai-*.sock` 文件前缀 | ❌ 不改 | 与目录名无关 |
| socket 路径计算逻辑 | ❌ 不改 | 仍是 `<注册表目录>/ai-<name>.sock` |
| `claude` agent 目录 | ❌ 不动 | 它目前只有 README + package.json，无实现代码 |

> 若后续决定连 `MNAB_REG_DIR` 一起改成 `AIBP_REG_DIR`，另起 D21，不在本次范围。

## 三、改动清单（共 12 处，逐条精确）

### 3.1 代码（3 处）

**改动 1** — `internal/aibp/registry.go`（第 26 行，`RegistryDir()` 返回值）

- 旧：
  ```go
  	return filepath.Join(base, fmt.Sprintf("microneo-agent-bridge-%d", os.Getuid()))
  ```
- 新：
  ```go
  	return filepath.Join(base, fmt.Sprintf("aibp-%d", os.Getuid()))
  ```

**改动 2** — `aibp-agents/pi/index.ts`（第 208 行，`registryDir()` 返回值）

- 旧：
  ```ts
      return path.join(base, `microneo-agent-bridge-${process.getuid?.() ?? 0}`);
  ```
- 新：
  ```ts
      return path.join(base, `aibp-${process.getuid?.() ?? 0}`);
  ```

**改动 3** — `aibp-agents/opencode/index.tsx`（第 471 行，`registryDir()` 返回值）

- 旧：
  ```tsx
      return path.join(base, `microneo-agent-bridge-${process.getuid?.() ?? 0}`)
  ```
- 新：
  ```tsx
      return path.join(base, `aibp-${process.getuid?.() ?? 0}`)
  ```

> 三处的 `registryDir()` 是孪生实现（opencode 注释自述「逐字复制 aibp-pi」），改完三端算法仍完全一致。

### 3.2 文档（6 个文件，8 处）

> CHANGELOG.md 已从清单移除，见上方改动 4 的撤销说明。

每处统一规则：**把路径里的 `microneo-agent-bridge` 子串替换为 `aibp`**。下表给精确锚点。

~~**改动 4** — `CHANGELOG.md`（第 21 行）~~

> ⚠️ **已撤销（不可追溯改写历史）**：CHANGELOG 是已发布版本的历史记录，属于不可变文档。即使其中含旧目录名 `microneo-agent-bridge`，也**保持原样**——历史条目应反映发布当时的原貌。如需说明改名，应在 changelog **新增**一条未来版本的记录，而非回溯改写旧条目。本仓库当前版本未到下一发布点，故本次改名**不在 CHANGELOG 做任何改动**。

**改动 5** — `docs/agent-comm/说明-AIBP.md`（第 95 行，§3.1 算法块）

- 旧片段：`dir  = base + "/microneo-agent-bridge-" + $UID`
- 新片段：`dir  = base + "/aibp-" + $UID`

**改动 6** — `docs/agent-comm/说明-AIBP.md`（第 128 行，§3.2 socket 示例）

- 旧片段：`"socket": "/tmp/.../microneo-agent-bridge-501/ai-pi-12345.sock",`
- 新片段：`"socket": "/tmp/.../aibp-501/ai-pi-12345.sock",`

**改动 7** — `docs/agent-comm/说明-发送端.md`（第 87 行）

- 旧片段：`return filepath.Join(base, fmt.Sprintf("microneo-agent-bridge-%d", os.Getuid()))`
- 新片段：`return filepath.Join(base, fmt.Sprintf("aibp-%d", os.Getuid()))`

**改动 8** — `docs/agent-comm/说明-接收端.md`（第 66 行）

- 旧片段：`return path.join(base, `microneo-agent-bridge-${process.getuid?.() ?? 0}`);`
- 新片段：`return path.join(base, `aibp-${process.getuid?.() ?? 0}`);`

**改动 9** — `docs/agent-comm/说明-接收端.md`（第 483 行，命令示例）

- 旧片段：`ls "$XDG_RUNTIME_DIR"/microneo-agent-bridge-*/`
- 新片段：`ls "$XDG_RUNTIME_DIR"/aibp-*/`

**改动 10** — `docs/agent-comm/D19-aibp-opencode.md`（第 28 行）

- 旧片段：`| registryDir | \`$XDG_RUNTIME_DIR/microneo-agent-bridge-$UID/\` | microNeo 端通过它发现 receiver |`
- 新片段：`| registryDir | \`$XDG_RUNTIME_DIR/aibp-$UID/\` | microNeo 端通过它发现 receiver |`

**改动 11** — `docs/agent-comm/README.md`（第 26 行）

- 旧片段：`**注册表 = 文件系统目录**：\`$XDG_RUNTIME_DIR/microneo-agent-bridge-$UID/ai-*.json\``
- 新片段：`**注册表 = 文件系统目录**：\`$XDG_RUNTIME_DIR/aibp-$UID/ai-*.json\``

**改动 12** — `aibp-agents/opencode/README.md`（第 50 行）

- 旧片段：`ls "$XDG_RUNTIME_DIR/microneo-agent-bridge-$(id -u)/"`
- 新片段：`ls "$XDG_RUNTIME_DIR/aibp-$(id -u)/"`

## 四、执行顺序

1. 先改 §三 的 12 处（代码 + 文档一起改，原子）。
2. 执行验证（§五）。
3. 报告结果，告知用户需要手动重启当前 pi session（§六）。

> 不需要 git commit（除非用户额外要求）。改动是纯字面量替换，无需 generate。

## 五、验证

执行以下命令，**全部应通过**：

1. **零残留检查**（最关键）—— 期望**无输出**：
   ```bash
   grep -rn "microneo-agent-bridge" \
     --include="*.go" --include="*.ts" --include="*.tsx" \
     --include="*.js" --include="*.md" --include="*.json" \
     . | grep -v node_modules
   ```
   若有输出，说明漏改，按 §三 清单补。

2. **新值在位检查** —— 期望**恰好 3 行**（3 端代码各 1 行）：
   ```bash
   grep -rn 'aibp-\${process.getuid\|aibp-%d\|`aibp-' \
     internal/aibp/registry.go aibp-agents/pi/index.ts aibp-agents/opencode/index.tsx
   ```
   （Go 用 `aibp-%d`，两个 TS 用反引号模板串 `aibp-`。）

3. **microNeo 编译**（用 `make build-quick` 跳过 generate）：
   ```bash
   make build-quick
   ```
   期望编译通过，无错误。

4. （可选）确认新目录算法可跑通：
   ```bash
   ls -la "${MNAB_REG_DIR:-${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/aibp-$UID}" 2>/dev/null \
     && echo "新目录已存在" || echo "新目录暂不存在（正常——需 receiver 重新注册）"
   ```

## 六、手动后续（提示用户，不由执行者操作）

- **当前机器上有个活着的 pi 注册在老目录**（`.../microneo-agent-bridge-501/ai-Alpha.json`）。改名并 `make build-quick` 后，microNeo 会去新目录找，**找不到**那个 pi。
- 解决：**重启那个 pi session**（让它用新代码重新注册到 `.../aibp-501/`）。或临时把老目录的 `ai-Alpha.json` + `ai-Alpha.sock` 挪到新目录（但 socket 路径已写死在 json 里，且老 socket fd 仍由老进程持有，最干净的还是重启）。
- **老目录残留无需清理**：`microneo-agent-bridge-<UID>` 在 runtime 目录（`$TMPDIR` / `$XDG_RUNTIME_DIR`），系统重启自动清。

## 七、风险

- **三端必须同步生效**：目录名是发送端发现接收端的唯一交汇点。只升一端 → 老端查老目录、新端查新目录，互相找不到 = 断连。本仓库内三端一起改、一起发版即可。开发期无外部用户，无平滑迁移负担。
- 无其他风险：改动是纯字面量，算法/协议/schema 均未触碰。
