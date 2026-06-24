# D19b · aibp-opencode 自动创建 Session

## 背景

当前 aibp-opencode 在 opencode 处于首页（无活跃 session）时收到 microNeo 的消息，会 toast「请先在 opencode 打开一个对话」并丢弃消息。

## 目标

aibp-opencode 收到消息时，如果没有活跃 session，**自动创建一个新对话**并递送消息，然后导航到该对话页面。

## 设计原则

**插件只负责递送消息，不干涉用户在 opencode 里的任何选择。** `session.create()` 不传 agent / model / workspace，由 opencode 服务端根据用户当前配置自动决定。

---

## 实施方案

### 改动文件

只改一个文件：`aibp-agents/opencode/index.tsx`

### 改动位置

`onMessage` 函数（约 115-138 行）：把当前「无 session → toast 回退」分支改为「无 session → 自动创建并递送」。

### 伪代码

```ts
// 新增：模块级状态
let pending: { sessionID: string; resolve: (id: string) => void } | null = null

async function ensureSession(): Promise<string> {
  const route = api.route.current as any
  if (route?.name === "session" && typeof route?.params?.sessionID === "string") {
    return route.params.sessionID
  }

  // 已经有消息在创建 session，等它完成，共享同一个 sessionID
  if (pending) {
    return await new Promise<string>((resolve) => pending!.resolve = resolve)
  }

  // 第一个进入创建流程的消息
  let resolveOuter!: (id: string) => void
  const sessionIDPromise = new Promise<string>((r) => { resolveOuter = r })
  pending = { sessionID: "", resolve: resolveOuter }

  try {
    const res = await client.session.create()
    if (res.error) {
      toast(`⚠ aibp 创建对话失败: ${res.error.message}`, "warning")
      throw new Error("session.create failed")
    }
    const sessionID = res.data.id
    pending.sessionID = sessionID
    api.route.navigate("session", { sessionID })
    return sessionID
  } finally {
    const p = pending
    pending = null
    p.resolve(p.sessionID)  // 通知等待中的消息
    // 注意：如果上面 throw 了，这里 resolve 空字符串，等待者会拿到空 ID 自行处理
  }
}

async function onMessage(env: any) {
  const text = formatText(env.payload)
  let sessionID: string
  try {
    sessionID = await ensureSession()
  } catch {
    return
  }
  if (!sessionID) return

  const res = await client.session.prompt({
    sessionID,
    parts: [{ type: "text", text }],
  })
  if (res.error) {
    toast(`⚠ aibp 消息发送失败: ${res.error.message}`, "warning")
  }
}
```

### 关键流程

| 场景 | 处理 |
|------|------|
| 消息到达，TUI 已在 session 页面 | 直接 `session.prompt()`，无需创建 |
| 消息到达，TUI 在首页，**且无其他消息正在创建** | `session.create()` → navigate → `session.prompt()` |
| 消息到达，TUI 在首页，**且已有消息在创建中** | 等待 `pending` 完成，共享同一个 sessionID，跳过 create + navigate |
| `session.create()` 失败 | toast 报错，等待中的消息也会拿到空 ID 自行退出 |
| `session.prompt()` 失败 | toast 报错（**当前代码只 log，需补 toast**） |

---

## 验收项

实施完成后必须实测：

1. 启动 opencode，停在首页
2. 在 microNeo 发送一条消息
3. 验证：
   - opencode 自动跳转到新创建的对话页面
   - 新对话里出现用户消息
   - LLM 开始回复
4. **并发测试**：快速连续发送 3-5 条消息，验证所有消息都被递送到同一个 session
5. **错误恢复**：验证当 `session.create()` / `session.prompt()` 失败时，toast 正确显示

---

## 技术验证

### opencode 服务端默认配置机制 ✅

经过源码验证（`packages/core/src/session/runner/model.ts`），opencode 服务端在处理无 agent/model 的 session 时有以下 fallback 机制：

```typescript
const selected = session.model
  ? // session 有 model，从 catalog 查找
    (yield* catalog.model.available()).find(...)
  : // session 无 model，使用 fallback：
    defaultModel && supported(defaultModel)
    ? defaultModel  // 优先使用用户默认配置
    : (yield* catalog.model.available()).find(supported)  // 或使用第一个可用 model
```

**结论**：零参数 `session.create()` 是安全的，服务端会自动使用用户默认配置或第一个可用 model。

### 剩余边界情况

| 场景 | 处理 |
|------|------|
| 用户没有任何配置的 provider | 服务端会报错 `ModelNotSelectedError`，这是合理的用户提示 |
| 快速连发多条消息 | ✅ 互斥锁 + 共享 sessionID 已处理 |

---

## 参考

- opencode Prompt 组件的 session 创建参考实现：`opencode/packages/tui/src/component/prompt/index.tsx` 的 `submitInner()` 函数
- SDK 方法：`OpencodeClient.session.create(parameters?)` / `session.prompt(parameters)`
- 插件 API：`packages/plugin/src/tui.ts` `TuiPluginApi` 类型定义
- **服务端默认配置逻辑**：`opencode/packages/core/src/session/runner/model.ts:203-221`
