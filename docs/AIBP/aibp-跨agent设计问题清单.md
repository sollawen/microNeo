# aibp 跨 Agent 设计问题清单

> 位置：`docs/agent-comm/aibp-跨agent设计问题清单.md`
> 维护者：sollawen
> 状态：**待修清单**（不在 v1 范围，修要 3 个 aibp 一起改）

---

## 0. 为什么需要这份文档

3 个 aibp 实现（pi / opencode / claude）独立演进，**累积了一些设计问题**：

| 类型 | 说明 |
|------|------|
| **B 类（Bug）** | 行为不正确，可能影响生产 |
| **I 类（不一致）** | 3 个实现行为不一样，容易踩坑 |

**修复原则**：
- ✅ **3 个 aibp 一起改**（避免引入新的不一致）
- ✅ 每个修复**都要补单测**（3 套测试都要更新）
- ❌ **不要单独修一个**——会产生新的不一致

---

## 1. 问题清单

### B1：`process.kill(pid, 0)` 在 EPERM 时误判为"进程已死" ⭐⭐⭐ P0

**现象**：
- `process.kill(pid, 0)` 用于检查进程是否存活（不发信号）
- 三种结果：成功 / `ESRCH`（进程不存在）/ `EPERM`（进程存在但**无权限**）
- 当前 3 个 aibp 都是 `catch { alive = false }`——**EPERM 被错误地当成"死了"**

**影响**：
- EPERM 时走 GC 分支 → `unlink(ai-<name>.json)` + `unlink(ai-<name>.sock)`
- **可能误删别人的活注册**（如果另一个 aibp 进程以无权限 PID 持有注册）

**出现频率**：
- 正常情况极低（PID 是当前用户启动的）
- 但**极端情况**（suid 程序 / namespace 隔离）可能触发
- 安全模型洁癖要求处理

**当前代码**（3 个 aibp 一致）：

```typescript
try { process.kill(pid, 0); alive = true; } catch { alive = false; }
//                                            ^^^^^^^^^^^^^^^^^^^
//                                            任何错误（包括 EPERM）都当死了
```

**修复方向**：

```typescript
try {
  process.kill(pid, 0);
  alive = true;
} catch (e: any) {
  if (e.code === 'EPERM') {
    // 进程存在但无权限 —— 视为"活"，不 GC
    alive = true;
  } else {
    // ESRCH 或其它 —— 真死了
    alive = false;
  }
}
```

**修复成本**：
- 改 3 处（`aibp-pi/index.ts` + `aibp-opencode/index.tsx` + `aibp-claude/aibp-channel.ts`）
- 每处 ~5 行
- 每个 aibp 加 1 个单测：mock `process.kill` 抛 EPERM

**影响范围**：低（3 个 aibp 的 v1 都不修）

---

### I1：aibp-opencode 的 `tryListen` 对 `EADDRINUSE` 也 log 刷屏 ⭐⭐ P1

**现象**：
- `tryListen` 失败时 error 事件触发
- aibp-pi：**EADDRINUSE 静默** + 其它 `console.warn`
- aibp-claude：复制 aibp-pi（**EADDRINUSE 静默** + 其它 `console.warn`）
- aibp-opencode：**所有错误都 `log()`**（包括 EADDRINUSE）

**影响**：
- 多 agent 场景下（pi + opencode + claude 共存），EADDRINUSE 是**正常竞争失败**（socket 已被别人占）
- aibp-opencode **每次都 log**——**多 agent 时刷屏**
- 用户体验差（终端噪音）

**当前代码对比**：

```typescript
// aibp-pi / aibp-claude（一致）
s.once("error", (err: NodeJS.ErrnoException) => {
  if (err.code !== "EADDRINUSE") {
    console.warn(`[aibp-pi] listen error on ${sockPath}: ${err}`);
  }
  finish(false);
});

// aibp-opencode（不一致——所有错误都 log）
s.once("error", (err: NodeJS.ErrnoException) => {
  log("listen error", { sockPath, code: err.code, message: err.message })
  finish(false)
})
```

**修复方向**：
- aibp-opencode 改成与 aibp-pi / aibp-claude 一致（EADDRINUSE 静默）

**修复成本**：
- 改 1 处（`aibp-opencode/index.tsx`）
- ~3 行
- 单测：mock listen 报 EADDRINUSE，验证**不**调用 `log()`

**影响范围**：低（v1 范围不修——但 opencode 用户体验差，**值得排进 v1.x**）

---

### B2：macOS 上 Unix socket 不强制 `EADDRINUSE` ⭐⭐⭐ P0

**现象**：
- Linux：`s2.listen(path)` 如果 path 已被 s1 占用 → 报 `EADDRINUSE`
- **macOS：s2.listen(path) 不会报 EADDRINUSE**——直接 listen 到**新的 socket 设备**
- `lsof` 验证：同一路径上**两个不同的 socket 设备**（不同 major:minor）

**实际影响**：
- aibp-pi / opencode / claude 的 `tryListen` 在 macOS 上**第一次 listen 就成功**（不 EADDRINUSE）
- **connect 探测分支（200ms 超时）不触发**——因为 tryListen 已经成功
- "真活进程" 检测代码（关键路径！）**在 macOS 上不执行**
- 实际行为：每次 listen 都成功，但多个进程都在同一路径 listen——**谁响应 client connect 不确定**

**实际触发**：
```bash
# macOS 下
$ bun -e 'import * as net from "node:net"; const s1 = net.createServer(()=>{}); await new Promise(r => s1.listen("/tmp/x.sock", r)); const s2 = net.createServer(()=>{}); const r = await new Promise(res => { s2.once("error", e => res({ok:false,code:e.code})); s2.once("listening", () => res({ok:true})); s2.listen("/tmp/x.sock"); }); console.log(r)'
# 输出：{ ok: true }   ← 期望 EADDRINUSE，实际不报
```

**影响范围**：
- **测试**：macOS 上无法稳定测"connect 探测真活"路径（aibp-claude 已删相关测试，注释说明）
- **生产**：macOS 用户多 agent 共存时，行为与 Linux 不一致——可能连接随机
- **CI 必需 Linux**——macOS 开发期不覆盖这些代码路径

**修复方向**（**v2 必做**）：

1. **短期**（v1 接受）：
   - 文档化：macOS 行为差异，CI 必须跑 Linux
   - 删 macOS 上无效的测试，注释指向此文档
   
2. **长期**（v2 改）：
   - 在 `tryListen` 之前先 `fs.statSync(sockPath)` 检查 socket 文件
   - 如果文件存在 + `tryListen` 没报 EADDRINUSE → 仍然走 connect 探测
   - 这样 macOS 上也能正确检测真活进程

**当前缓解**：
- aibp-claude 删除了"connect 探测真活"测试（macOS 不稳定）
- aibp-claude 协议层覆盖率 98.73% 接受（剩下 2 行是 zombie 分支，macOS 难测）

**3 个 aibp 影响**：理论上都有，**实际只在 CI 跑 Linux 时能验证**。



---

## 2. 暂不修（设计选择，与 aibp-pi 一致保留）

### D1：损坏的 `ai-*.json` 不 GC

- aibp-pi：损坏 JSON → `continue`（不 GC，不删用户文件）
- aibp-opencode / aibp-claude：与 aibp-pi 一致

**为什么不改**：损坏可能是文件系统问题（fsync 失败、磁盘满）—— 自动删除会**掩盖问题**。让用户手动处理。

### D2：connect 探测 200ms 超时硬编码

- 3 个 aibp 都是 200ms 硬编码

**为什么暂不改**：当前是合理值；提取为常量收益小（一个文件就 1-2 处用到）。如果未来有可调需求再抽。

### D3：JSON 损坏的日志级别

- 3 个 aibp 都是 `catch { continue }`——**完全静默**
- 不打印 warn

**为什么暂不改**：与 D1 一致（不 GC 也不警告）—— 哲学是"不打扰用户"。如果用户想知道，应该用文件系统工具自己看。

---

## 3. 修复节奏

### 短期（v1 不修，但记录）
- B1（EPERM 误判）—— 写在这里等 v2 一起修
- I1（aibp-opencode 刷屏）—— 排进 v1.x 单独修

### 中期（v2 一起修）
- 3 个 aibp 加 `EPERM` 单独处理（B1）
- 3 个 aibp 同步 tryListen 错误处理（I1 + 防止未来漂移）

### 长期（v2+）
- 把硬编码的 200ms 提取为常量（D2）
- 抽公共代码到 `@microNeo/aibp-shared` 包（避免三处独立维护）

---

## 4. 验证清单

每个 bug 修复 PR 必须包含：

- [ ] 3 个 aibp 代码都改（`aibp-pi/index.ts` + `aibp-opencode/index.tsx` + `aibp-claude/aibp-channel.ts`）
- [ ] 3 个 aibp 测试都加（B1 加 EPERM 用例；I1 加 EADDRINUSE 不 log 用例）
- [ ] CI 跑通 3 套测试
- [ ] PR 描述里 cross-link 这份文档 + 方案（避免 reviewer 困惑）

---

## 5. 历史

| 日期 | 事件 |
|------|------|
| 2026-07-02 | 初稿：B1（EPERM 误判）+ I1（aibp-opencode 刷屏）+ B2（macOS Unix socket 行为差异）3 个问题归档 |
