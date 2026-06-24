# D19-aibp-opencode · 源码核对（针对 `../opencode/` @ v1.17.9）

> **目的**：把 D19 的关键假设逐条对 opencode 源码核对，给出「**最简洁且有效**」的落地方案与必要修正。
> 结论先行：**D19 的核心选型（server 插件 + `client.tui.*`）完全成立，全链路已在源码中验证**。但有 4 处实现细节需要修正/补强（见 §二），不修正会有边缘失败。

---

## 一、已逐行验证为正确的部分（D19 无需改动）

### 1.1 `client.tui.*` 存在且语义如 D19 所述
`packages/sdk/js/src/gen/sdk.gen.ts:1032-1134` + `:1194`，`OpencodeClient.tui` 命名空间提供：

| 方法 | HTTP | 服务端行为 |
|------|------|-----------|
| `appendPrompt({body:{text}})` | POST `/tui/append-prompt` | 发 `tui.prompt.append` 事件 |
| `submitPrompt()` | POST `/tui/submit-prompt` | 发 `tui.command.execute {command:"prompt.submit"}` |
| `clearPrompt()` | POST `/tui/clear-prompt` | 发 `tui.command.execute {command:"prompt.clear"}` |
| `showToast({body:{message,variant}})` | POST `/tui/show-toast` | 发 `tui.toast.show` 事件 |

→ 服务端 handler：`packages/opencode/src/server/routes/instance/httpapi/handlers/tui.ts`。

### 1.2 「TUI 模式进程内可达」铁证（D19 §2.2）
`packages/opencode/src/plugin/index.ts` 构造 client：
```ts
const serverUrl = Server.url                       // undefined（TUI 模式从不 listen）
const client = createOpencodeClient({
  baseUrl: serverUrl?.toString() ?? "http://localhost:4096",
  ...(serverUrl ? {} : { fetch: (...args) => Server.Default().app.fetch(...args) }),
})
```
`packages/opencode/src/server/server.ts:70`：`export let url: URL | undefined` —— 初始 `undefined`，仅 `listen()`（显式 TCP）时才赋值。**TUI 模式不 listen → `url` 恒 `undefined` → client 用 in-process `fetch` 直连 Hono app**。`client.tui.*` 是纯进程内调用，不依赖任何端口。✅ D19 §2.2/§5.1 结论正确。

### 1.3 TUI 侧确实订阅并响应这些事件（闭环）
- `appendPrompt` → `packages/tui/src/component/prompt/index.tsx:233`：`input.insertText(evt.properties.text)`（**在光标处插入，非替换**）。
- `submitPrompt`/`clearPrompt` → `packages/tui/src/app.tsx:955`：`keymap.dispatchCommand("prompt.submit"/"prompt.clear")` → 命令 `run()`（`prompt/index.tsx:336/344`，`submit()` 即「按回车」→ 触发 LLM）。
- `showToast` → `packages/tui/src/app.tsx:962`：`toast.show(...)`。
- **所有事件都被 `workspace !== project.workspace.current()` 过滤** → 只作用于当前活动 workspace/session。✅ 这正是 D19 V4「路由到活动会话」的机制根源，无需我们自己定位 session。

### 1.4 `clearPrompt + appendPrompt` 模式正确（D19 §3.7）
因为 `appendPrompt` = `insertText`（追加/插入），不是 set。要先 `clearPrompt` 再 `appendPrompt` 才能达成「替换」语义。✅

### 1.5 配置键名 = `"plugin"` 数组（D19 §6.2）
`packages/opencode/src/config/config.ts:113` 内部字段 `plugin_origins`，对用户 `opencode.json` 的键是 **`"plugin"`**（`install.ts:101` 读写 `item.plugin`）。D19 写 `{ "plugin": ["aibp-opencode"] }` 正确。

### 1.6 安装命令存在（D19 §6.2 / §七）
`packages/opencode/src/cli/cmd/plug.ts`：`PluginCommand` = `plugin <module>`，alias `plug`，`-g/--global`（全局配置）、`-f/--force`（替换）。`opencode plugin aibp-opencode -g` 真实可用。✅

### 1.7 `main: "./index.ts"` 可被 server 插件正确解析（D19 §6.1）
`shared.ts` 的 `resolvePackageEntrypoint`：server kind 先找 `exports["./server"]`，找不到则回退 `pkg.main`。D19 的 `main:"./index.ts"` 路径成立（无需 `exports["./server"]`）。✅

---

## 二、需要修正/补强的 4 处（源码核对后发现）

### 2.1 ★ 插件导出形态：用 `{ server: fn }`，不要 bare default（修正 D19 §6.3 / §五）

**D19 现状**：声称 `export default async function plugin(input){…}`（bare default）会被「零歧义」识别为 server 插件，依据是「opencode-browser 这么做」。

**源码真相**（`packages/opencode/src/plugin/index.ts:applyPlugin`）：插件加载分两条路，**bare default 走的是 legacy fallback，不是 first-class 路径**：

```ts
async function applyPlugin(load, input, hooks) {
  const plugin = readV1Plugin(load.mod, load.spec, "server", "detect")  // ① first-class
  if (plugin) {
    await resolvePluginId(...)
    hooks.push(await plugin.server(input, load.options))
    return
  }
  for (const server of getLegacyPlugins(load.mod)) {                    // ② legacy fallback
    hooks.push(await server(input, load.options))
  }
}
```

- **bare default（D19 现案）**：`readV1Plugin` 在 detect 模式下见 `mod.default` 非对象 → 返回 `undefined` → 落入 `getLegacyPlugins`。`getLegacyPlugins` 遍历 **所有** `Object.values(mod)`，**任一导出不是函数 / 非 `{server}` 对象就抛 `"Plugin export is not a function"`**。能跑，但：① 走 legacy；② **强约束「文件里不能有任何非函数具名导出」**（一个常量/类型导出就炸）。
- **`{ server: fn }`（推荐）**：直接走 first-class v1 路径，匹配 `PluginModule` 类型定义（`packages/plugin/src/index.ts`：`PluginModule = { id?; server: Plugin; tui?: never }`）。对具名导出无约束。

**修正建议**：导出改成
```ts
export default {
  id: "aibp-opencode",          // 见 §2.2，文件式加载需要
  server: async function (input) {
    // 原 plugin 函数体
    return { dispose: cleanup } // 见 §2.3
  },
}
```

> 与 aibp-pi 的「同构度」并未受损：仍是单 default export、无 JSX、delivery 仍是一组 API 调用；只是包了一层 `{server:…}` 对象。这是 opencode 当前推荐的 `PluginModule` 形态，不是倒退。

### 2.2 ★ 文件式加载必须导出 `id`（D19 §6.2 漏写）

**源码**（`shared.ts:resolvePluginId`）：
```ts
if (source === "file") {
  if (id) return id
  throw new TypeError(`Path plugin ${spec} must export id`)   // ← 文件式无 id 直接抛错
}
// npm：id 可选，回退到 package.json 的 name
```
- npm 安装：`id` 可省，自动用 `package.json.name`。
- **文件式**（`~/.config/opencode/plugins/aibp-opencode.ts`，D19 §6.2 开发期方案）：**必须在 default 对象里带 `id`**，否则加载失败。

**修正建议**：default 对象统一带 `id: "aibp-opencode"`，npm / 文件两种加载方式都能用。

### 2.3 用 `dispose` Hook 做清理，优于仅靠 `process.once("exit")`（增强 D19 §3.4）

**源码**：opencode 在 Layer finalizer 里**显式调用每个 hook 的 `dispose?.()`**（`index.ts` 末段 `Effect.forEach(hooks, h => h.dispose?.())`）。`Hooks` 类型已声明 `dispose?: () => Promise<void>`。

**修正建议**：plugin 返回 `{ dispose: cleanup }` 而非空 `{}`。`process.once("beforeExit"/"exit")` 作为兜底保留（应对 kill -9）。这样 opencode 正常退出时 server 关闭 / 注册文件删除由框架驱动，更可靠。

### 2.4 package.json 加 `engines.opencode`（落实 D19 R5）

**源码**（`shared.ts:checkPluginCompatibility`）：**npm 插件**若声明 `engines.opencode` 会被 semver 门控（不符则跳过并报错）；不声明则不门控。文件式跳过门控。

**修正建议**：`package.json` 加
```json
"engines": { "opencode": ">=1.4.0" }
```
（具体下限随 `@opencode-ai/plugin`/`sdk` 的 devDep 下限对齐；当前核对仓库版本 1.17.9。）
这把 D19 §十 R5「opencode 升级改 `client.tui.*` 契约」的风险从「静默坏掉」变成「加载期明确拒绝」，值得加。

---

## 三、核对后推荐的最终形态（差异清单）

相对 D19 §五骨架，**仅这 4 处不同**，其余（协议常量、registryDir、名字池、formatText、分帧、版本校验、报文处理、递送语义）全部逐字保留：

| 位置 | D19 原案 | 核对后推荐 |
|------|---------|-----------|
| 导出形态（§6.3/§五） | `export default async function plugin(input)` | `export default { id:"aibp-opencode", server: async (input)=>{…} }` |
| id（§6.2） | 未提 | default 对象带 `id`（文件式加载必需） |
| 清理（§3.4） | 仅 `process.once("exit")`，返回 `{}` | 返回 `{ dispose: cleanup }`，`process.once` 作兜底 |
| package.json（§6.1） | 无 `engines` | 加 `"engines":{"opencode":">=1.4.0"}` |

**代码量影响**：+3 行（对象包一层 + id 字段 + dispose 返回）。换来：走 first-class 加载路径、文件式可加载、框架驱动清理、版本门控。**完全符合「最简洁且有效」**。

---

## 四、加载方式确认（D19 §6.2 无误，补充细节）

两种 opencode 原生方式都可用：
1. **npm**：`opencode plugin aibp-opencode -g`（写全局 `~/.config/opencode/opencode.json` 的 `plugin` 数组）→ 重启。
2. **文件式（开发期）**：放 `~/.config/opencode/plugins/aibp-opencode.ts`，**必须带 `id`**（§2.2）。Bun 直接加载 `.ts`，无需预编译（D19 V5 已验，源码无 build 强制）。

> `resolvePathPluginTarget`（`shared.ts`）：文件式若指向目录，目录里**要么有 `package.json`、要么有 `index.{ts,tsx,js,mjs,cjs}`**，否则报 `missing package.json or index file`。单文件 `.ts` 直接可用。

---

## 五、风险复核（针对源码）

| D19 风险 | 核对结论 |
|---------|---------|
| R1（appendPrompt+submit 不触发 LLM） | **机制已闭环**（§1.2/§1.3），不存在 |
| R2（streaming 中 submit 行为） | 源码未特殊处理：`prompt.submit` 命令的 `run()` 调 `submit()`，opencode 自行排队/拒绝。仍建议 V2 实测，但非阻塞 |
| R5（opencode 升级改 `client.tui.*` 契约） | **§2.4 的 `engines.opencode` 把它变成加载期硬门控**，可缓解 |
| —（新增）bare default + 误加非函数具名导出 → legacy 路径抛错 | **§2.1 改用 `{server:fn}` 根除** |
| —（新增）文件式加载忘带 `id` → 抛 `Path plugin must export id` | **§2.2 统一带 `id` 根除** |

---

## 六、一句话总结

D19 选对了路（server 插件 + `client.tui.*`，全链路源码可证、进程内直达 TUI 输入框）。落地时只需把导出从 bare function 收成 `{id, server}` 对象、补 `dispose`、加 `engines.opencode` 三处微调，即是最简洁稳健的 plugin 形态。可据此进入 Phase 1 实现。
