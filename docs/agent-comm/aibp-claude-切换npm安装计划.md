# aibp-claude：从源码目录安装 → npm 包安装（切换计划）

> 目标：把本机 Claude 里 aibp-claude 的加载方式，从 `ccaibp` 带 `--plugin-dir <源码路径>`（session-only），
> 换成经 marketplace + npm 的持久安装——之后任何 `claude` 启动都自动加载，`ccaibp` 退化为纯 env 中转。
>
> 关键文档依据：`internal/aibp/aibp-agents/claude/README.md`、`分发安装方式分析.md`、
> `reference/claude-plugin-doc.md`（§Plugin caching："Through `claude --plugin-dir` … for the duration of a session"）。

> 🔒 **硬约束：不走 Claude 官方 plugin-market**
>
> 本计划全程使用**自建 marketplace**（`sollawen/microNeo-plugins`，GitHub 公开仓库）。
> 绝不提交 / 不依赖 `@claude-plugins-official`、`@claude-plugins-community`（不需审核、不与 Anthropic 绑定）。
> 这与 `分发安装方式分析.md` §3 第 5 行的排除决策一致。
>
> ⚠ 注意：本机 `known_marketplaces.json` 里现存的 `claude-plugins-official` 是给 `superpowers` 插件用的，**与 aibp 无关，本次不动它**。

---

## 0. 现状确认（已查清，无需动手）

| 项 | 状态 | 说明 |
|---|---|---|
| **当前装法** | `~/.zshrc` 第 122 行 `ccaibp()` 里 `claude --plugin-dir /Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude "$@"` | session-only flag，**无任何持久化痕迹** |
| `installed_plugins.json` | 仅 `superpowers@claude-plugins-official`，**无 aibp-claude** | 即「根本没装过」，无需 `uninstall` |
| `known_marketplaces.json` | 仅 `claude-plugins-official`，**无 microNeo-plugins** | 待加 |
| `enabledPlugins`（settings.json） | 仅 superpowers | 装完会自动写入 |
| npm `aibp-claude` | **已发布 `1.0.0`**（4 文件：`plugin.json` / `index.ts` / `package.json` / `README.md`，零依赖） | ✅ 交付层就绪 |
| GitHub `sollawen/microNeo-plugins` | **公开仓库，main 分支已含 `.claude-plugin/marketplace.json`**（`source:npm`，无 `version` = 跟最新） | ✅ 目录层就绪 |
| Claude 版本 | `2.1.199`（≥ 2.1.105，支持 Monitor） | ✅ |
| `bun` | `1.3.6`（monitor 用它跑 `index.ts`） | ✅ |
| 残留 registry | `$TMPDIR/aibp-<uid>/ai-Alpha.json` + `.sock`（上次 session 的僵尸文件） | 无害：下次启动 `allocateName` 的 GC 会自动清。预期行为，见 README 注意事项 1 |

> **一句话**：所有交付/目录/运行时依赖都已就位，本机只是「还没登记 marketplace、还没 install」。切换纯增量。

---

## 1. 关键认知：为什么"删除旧 aibp"等于"改一行 .zshrc"

官方文档（`reference/claude-plugin-doc.md` §Plugin caching）明确：

> Plugins are specified in one of two ways:
> * Through `claude --plugin-dir` or `claude --plugin-url`, **for the duration of a session**.
> * Through a marketplace, installed for future sessions.

即 `--plugin-dir` 是**每次 session 临时加载**，Claude 退出即无痕——它**不会**写 `installed_plugins.json`、不进 cache、不留 enabledPlugins。

实测验证：`cat ~/.claude/plugins/installed_plugins.json` → 只有 `superpowers`，确实无 aibp-claude。

所以：
- ❌ 不存在"先 `claude plugin uninstall`"这一步（没装过，uninstall 会报 not found）。
- ❌ 不存在"删 cache 目录"这一步（cache 里没有）。
- ✅ 只需：装 npm 版 + 把 `.zshrc` 里那行 `--plugin-dir ...` 去掉。

源码目录 `/Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude` **保留不动**（master 分支的发布源头，`npm publish` 还要用）。

---

## 2. 执行步骤（按序，每步可独立验证）

### 步骤 1️⃣：登记自建 marketplace（一次性）

```bash
claude plugin marketplace add sollawen/microNeo-plugins
```

- Claude 会 clone `github.com/sollawen/microNeo-plugins` 到 `~/.claude/plugins/marketplaces/microNeo-plugins/`，读 `.claude-plugin/marketplace.json`。
- 验证：`claude plugin marketplace list` 应见 `microNeo-plugins`（source: github / sollawen/microNeo-plugins）。
- 同步检查 `known_marketplaces.json` 多出 `microNeo-plugins` 条目。

### 步骤 2️⃣：安装插件（一次性，默认 user scope）

```bash
claude plugin install aibp-claude@microNeo-plugins
```

- Claude 解析 marketplace 条目 → `source: {source:"npm", package:"aibp-claude"}` → 从 npm registry 拉 `aibp-claude@1.0.0` tarball → 解压复制到 `~/.claude/plugins/cache/microNeo-plugins/aibp-claude/1.0.0/`。
- 自动写入：`installed_plugins.json` + `enabledPlugins`（user scope，`~/.claude/settings.json`）。
- 验证：
  ```bash
  claude plugin list                 # 应见 aibp-claude@microNeo-plugins  enabled
  ls ~/.claude/plugins/cache/microNeo-plugins/aibp-claude/   # 应有 1.0.0/
  ```

### 步骤 3️⃣：改 `.zshrc`——`ccaibp` 去掉 `--plugin-dir`

**改前**（`~/.zshrc` 第 122 行）：
```bash
    claude --plugin-dir /Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude "$@"
```

**改后**：
```bash
    claude "$@"
```

> 改完 `ccaibp` 就是纯 env 中转（`ANTHROPIC_BASE_URL` / `_AUTH_TOKEN` / `_MODEL` 等），与 README「第三方 LLM 中转（ccmm 风格）」一致——插件已持久安装，任何 `claude` 启动自动加载，无需再带 flag。

生效：`source ~/.zshrc`（或开新 terminal）。

### 步骤 4️⃣：端到端验证（必须开新 session 实测）

```bash
ccaibp        # 起 Claude 会话（现在不带 --plugin-dir）
```

会话内 / 起来后逐项核对：

1. **`/plugin list`** → 见 `aibp-claude@microNeo-plugins` 且 enabled。
2. **monitor 自动 spawn**（这是方案命门）：
   ```bash
   # 另开一个 terminal，应在 ccaibp 启动后看到 daemon 子进程
   ps aux | grep 'bun.*index.ts' | grep -v grep
   ```
   见到 `bun ".../cache/microNeo-plugins/aibp-claude/1.0.0/index.ts"` 进程。
3. **registry 写入**：
   ```bash
   ls "$TMPDIR/aibp-$(id -u)/"        # 见 ai-<Name>.json（如 ai-Bravo.json）
   tail /tmp/aibp-claude.log          # 日志应有 boot / allocateName 记录
   ```
4. **Claude 界面 task panel / monitor 列表** → 显示 `aibp-daemon`（描述 "microNeo editor context & messages (AIBP)"）。
5. **端到端触发**（核心目标 §6.3）：
   - microNeo 里打开一个文件、选中代码、按 `Alt-Enter`（带或不带消息）。
   - Claude 主对话区应 **立即开始 streaming 响应**（界面先闪一行 `Monitor event: "microNeo editor context & messages (AIBP)"`，紧接 LLM 回复）。
   - 无需再按回车；这就是 Monitor "interject" 的核心保证。

---

## 3. 可能的注意点

### 3.1 Bash 权限（实测：不需要）

当前 `~/.claude/settings.json`、`~/.claude.json`、项目级、local 级**均无任何 `permissions` 段**，但 `ccaibp` 的 monitor 照样正常 spawn（日志里多个 daemon 都正常 emit 过事件）。这证明 **monitor 的 spawn 不走 Bash 权限门禁**——它是 Claude 按 `plugin.json` 声明起的子进程，不是 LLM 调 Bash 工具，不受 `permissions.allow` 管辖。

切到 marketplace 安装后行为一致：插件一经 install/enable（=用户明确同意）→ monitor 预信任。**预期不弹权限提示，无需预置 allow 规则。**

> 极小概率仍有弹窗的兑底方案（不用预置，弹了再加）：
> ```jsonc
> { "permissions": { "allow": ["Bash(bun \"${CLAUDE_PLUGIN_ROOT}/index.ts\")"] } }
> ```
> 加到 `~/.claude/settings.json` 顶层即可，别覆盖现有字段。

### 3.2 残留 registry（**只**在异常退出时出现，正常退出干净）

**正常退出不留残渣**——`index.ts:336-341` 的信号清理链已验证：

```js
process.on('SIGTERM', () => process.exit(0))   // Claude 正常退出时发 SIGTERM
process.on('SIGINT',  () => process.exit(0))
process.on('exit', () => {                       // process.exit(0) 同步触发
  try { fs.unlinkSync(regFile) } catch {}
  try { fs.unlinkSync(socketPath) } catch {}
})
```

即 `Claude 退出 → SIGTERM → process.exit(0) → exit handler → unlink(regFile) + unlink(socketPath)`。实测 `ccaibp` 正常 `:exit` / `/quit` / Ctrl-D 后，`$TMPDIR/aibp-<uid>/` 是空的。

> ⚠ README「注意事项 1」把它写成「退出残留（预期）」是**误导**——那暗示残留是常态，实际正常退出完全干净。本节以代码实测为准。（README 那条建议后续单独修，属 master 分支源码改动。）

**只有绕过 SIGTERM/SIGINT 的异常终止才会留残渣**，例如：

- 直接关 terminal 窗口 → 给进程组发 **SIGHUP**，而 daemon **没注册 SIGHUP handler**（`index.ts` 只有 SIGPIPE/SIGTERM/SIGINT）→ Node 默认终止 → `exit` 不触发 → 不清理；
- Claude 崩溃 / `kill -9` / 机器硬重启。

当前看到的那个 `ai-Alpha.json`（pid 8655，日志尾无任何 cleanup 记录）就是这类异常终止留下的。无害：下次任一 AIBP agent 启动时 `allocateName` 的 GC 会探测 pid 存活、死的 unlink。正常用不用管它。

> （可选优化，**非本计划范围**）给 `index.ts` 补一行 `process.on('SIGHUP', () => process.exit(0))`，关 terminal 窗口也能走正常清理——源码改动，需单独许可。

### 3.3 `--plugin-dir` 与 npm 版同名的优先级

本次已通过改 `.zshrc` 彻底去掉 `--plugin-dir`，不存在冲突。即便将来临时用 `claude --plugin-dir <path>` 测本地改动，按官方设计**本地目录在该 session 优先**（便于测已装插件的改动），无需先卸 npm 版。

---

## 4. 回滚预案（万一 npm 版起不来）

按需从轻到重：

1. **临时回退 session 加载**：单次起会话带 `--plugin-dir`（不改 .zshrc）：
   ```bash
   ccaibp --plugin-dir /Users/sollawen/pi-dev/microNeo.master/internal/aibp/aibp-agents/claude
   # 但注意 ccaibp 现已无 --plugin-dir，"$@" 透传，这条会拼到 claude 后面
   ```
2. **`.zshrc` 还原**：把第 122 行加回 `--plugin-dir ...`（git 未追踪 .zshrc，手动还原即可）。
3. **禁用但不卸**（保留 cache 以便排查）：`claude plugin disable aibp-claude@microNeo-plugins`。
4. **彻底卸**：
   ```bash
   claude plugin uninstall aibp-claude@microNeo-plugins
   claude plugin marketplace remove microNeo-plugins
   ```
   再把 `.zshrc` 加回 `--plugin-dir`。npm 包与 GitHub marketplace repo 不受影响。

---

## 5. 完成后的后续发版节奏（仅记录，本次不做）

之后 microNeo 发新版 aibp-claude：

```bash
cd internal/aibp/aibp-agents/claude
# bump package.json: 1.0.0 → 1.0.x
npm publish
```

- marketplace.json 的 `source` 未设 `version` → 自动跟最新，**无需改 GitHub repo**。
- 本机拿新版：
  - 手动：`claude plugin update aibp-claude@microNeo-plugins`
  - 自动：交互界面 `/plugin` → Marketplaces → `microNeo-plugins` → Enable auto-update

---

## 附：本次需要用户授权的操作（PLAN 模式受限）

| 操作 | 性质 | 是否需许可 |
|---|---|---|
| 步骤 1/2 两条 `claude plugin` 命令 | bash 执行（非代码改动） | ✅ 需用户确认执行 |
| 步骤 3 改 `~/.zshrc` | 配置文件（非 .md/.txt） | ✅ 需用户许可 |
| 步骤 4 验证命令（`ccaibp`、`ps`、`ls`、`tail`） | bash 执行 | ✅ 需用户确认 |

> 本计划文档本身是 `.md`，PLAN 模式下可直接写。落地执行等用户许可后逐项进行。
