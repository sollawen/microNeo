# D23 — aibp 扩展的自动升级机制（D20 改名引发的讨论）

> **状态**：🟡 开放讨论 / 草稿（2026-06-24 晚记，待明天继续）
>
> **来源**：执行 D20（注册表目录改名）时发现 —— 改完 microNeo 端代码后，receiver 端（pi）装的是 npm 包旧副本，源码改动对它无效；进而引出「能否用 `:check-agent` 自动升级」的讨论。
>
> **不是**：本文不做实现，只把需求、现状、证据、问题摆清，供决策。
>
> **相关**：`D20-aibp-regdir-rename.md`（改名本身）、`说明-AIBP.md §7`（协议版本语义）、`internal/aibp/ensure_agents/ensure.go`（现有实现）。

---

## 一、今天发生了什么（背景）

1. **D20 改名任务已执行完毕**：注册表目录 `microneo-agent-bridge-<UID>` → `aibp-<UID>`。3 处代码 + 9 处文档全改，`make build-quick` 通过，零残留。
2. **改完发现 receiver 端的生效方式不一致**：

| agent | 加载方式 | 当前生效的代码 | 改名后状态 |
|-------|---------|---------------|-----------|
| **opencode** | 源码路径（`aibp-agents/opencode/`） | ✅ D20 改的新代码 | ✅ 重启 opencode 即生效 |
| **pi** | **npm 包** `aibp-pi@1.0.1`（`~/.pi/agent/npm/node_modules/aibp-pi/`） | ❌ 装的是改名前的旧副本 | ❌ 源码改动没进 npm 包，对运行中的 pi 无效 |

3. **pi 的加载链证据**（铁证）：
   - `~/.pi/agent/settings.json`:6 → `"npm:aibp-pi"`（按 npm 包加载）
   - `~/.pi/agent/npm/package.json` → `"aibp-pi": "^1.0.1"`
   - 实际装的位置 `~/.pi/agent/npm/node_modules/aibp-pi/index.ts:208` → **仍是旧的** `microneo-agent-bridge-...`
   - npm registry 最新版 = `1.0.1`（从未发过含改名的新版）

4. **引出核心需求**：希望改了协议/实现后，用户能用 `:check-agent` 一键自动更新 receiver 扩展，免手动 `pi update npm:aibp-pi`。

---

## 二、现状：`:check-agent` 的真实能力边界

### 2.1 它实际做什么

入口 `internal/action/command_neo.go:CheckAibpCmd` → 遍历 `AllEnsurers` → 每个已装 agent 跑 `ensure_agents.Ensure()`。

`Ensure()`（`internal/aibp/ensure_agents/ensure.go:47`）的编排：

```
HasAgent?  ─ no ─→ 报错返回（agent 没装）
   │ yes
HasAIBP?   ─ no ─→ 【会自动装】 InstallAIBP()              ← 触发点 ①
   │ yes
AIBPVersion() error? ─ yes ─→ 【会自动重装】 InstallAIBP()  ← 触发点 ②
   │ no（读到了 aibp.protocol）
比较 协议主版本：
   ext == mine ─→ 报 "ready"，返回                         ← ✅ 常态
   ext <  mine ─→ 报 "outdated, please upgrade"，返回      ← ⚠️ 只报错，不装
   ext >  mine ─→ 报 "microNeo outdated"，返回
```

### 2.2 两个关键事实（颠覆最初假设）

**事实 1：`:check-agent` 比较的是协议版本，不是 npm 包版本。**
- `AIBPVersion()`（`ensure_pi.go:80`）读的是 package.json 的 `aibp.protocol` 字段（值如 `"aibp-1"`），**不是** `"version"` 字段（`1.0.1`）。
- 所以「本地 1.0.1 vs npm 1.0.2」这种包版本差异，`check-agent` 根本不看。

**事实 2：即使协议版本判为「过旧」，也不自动安装。**
- `ensure.go:83-85`：`outdated` 分支只有 `report(...)` + `return errExtensionOutdated`，**`InstallAIBP()` 一次都没调**。
- 所以即便升协议版本号也救不了，自动更新这条链是断的。

### 2.3 `:check-agent` 会自动调 `InstallAIBP()` 的，只有这两种情况

| 情况 | 触发条件 |
|------|---------|
| ① 首次安装 | `HasAIBP() == false`（settings.json 里没登记 `npm:aibp-pi`） |
| ② 损坏自愈 | `AIBPVersion()` 返回 error（package.json 缺失/损坏/无 `aibp.protocol` 字段） |

> 「版本旧但读得出来」这种情况，被当前设计**故意排除在自动安装之外**——只报错让用户手动升。

---

## 三、问题与矛盾

### 3.1 核心矛盾（明天讨论的焦点）

用户想要的「自动更新」 vs `:check-agent` 实际做的「协议一致性体检 + 首装/自愈」——**两套目标不重合**。

而 D20 改名恰好落在两者缝隙里：
- **协议层**：目录改名是纯实现细节，`aibp.protocol` 仍是 `"aibp-1"`，**没变**。
- **实现层**：receiver 确实需要换上新代码才能和新 microNeo 对上目录。

结果：协议体检说「ready ✅」（都是 aibp-1），但**实际通信已经断了**（两端去不同目录找）。`:check-agent` 对这种情况完全失明。

### 3.2 为什么「升协议版本号」也不是好办法

用户提过：能不能把 `aibp-1` 升成 `aibp-2` 来驱动自动更新？两个独立的问题：

1. **技术上仍不通**（§2.2 事实 2）：升了协议号，`outdated` 分支照样只报错不安装。必须**同时**改 `ensure.go` 在该分支补 `InstallAIBP()`。
2. **语义上是错的**（`说明-AIBP.md §7.1`）：主版本 +1 的定义是「报文结构不兼容变更」。目录改名零协议语义变化。用协议号当实现升级触发器，会**污染版本号的语义**——以后看到 `aibp-2`，分不清是真协议改了，还是只是实现细节变了。

### 3.3 两套版本号的混淆

| | npm 包 `version` | 协议 `aibp.protocol` |
|---|---|---|
| 例子 | `1.0.1`, `1.0.2` | `aibp-1`, `aibp-2` |
| 所属层 | **实现层**（包发布） | **契约层**（报文兼容性） |
| 变更频率 | 高（每次发版） | 极低（协议不兼容才变） |
| `:check-agent` 当前看哪个 | ❌ 不看 | ✅ 只看这个 |
| 改名属于哪个层 | ✅ 属于这个 | ❌ 不属于 |

「自动更新 receiver 实现」本质是个**实现层**问题，理应由 npm `version` 驱动，不该借道协议号。

---

## 四、待决策（明天要定）

### 选项 A：接受现状，改名走手动更新

- D20 的 aibp-pi 改名发成 `aibp-pi@1.0.2`（**协议号仍 `aibp-1`**）。
- 用户手动 `pi update npm:aibp-pi`。
- `:check-agent` 这次帮不上忙，文档里说明即可。
- **成本**：最低。零新代码。
- **代价**：用户得手动操作一次；且未来每次实现层变更都得手动。

### 选项 B：扩展 `:check-agent`，加「实现层版本跟进」

让 `:check-agent` 在协议一致（ready）之后，**再**比较 npm 包 `version`：本地 < registry → 自动 `InstallAIBP()`。

- **技术要点**：
  - `InstallAIBP` 本来就 unpinned（`ensure_pi.go:24` 注释 D5，`pi install npm:aibp-pi` 不带 `@版本`）→ 装的就是最新。所以「升级」其实已有现成机制，**真正缺的只是「判断要不要升级」+「在 ready 后触发它」**。
  - 判据需要联网查 registry：`npm view aibp-pi version`（~1-2s）。`:check-agent` 本来就联网几秒，延迟可接受。
  - opencode 是源码加载，无 npm version 概念 → 该机制只对 pi 类（npm 包加载的）agent 有意义。需在 `AgentEnsurer` 接口加可选方法（如 `NpmVersion()` / `LatestVersion()`），pi 实现它，opencode 返回「不适用」。
- **成本**：中等。新增接口方法 + pi 实现 + 联网查询 + 测试。另起实施文档。
- **收益**：一劳永逸——以后任何实现层变更，用户 `:check-agent` 自动跟进。
- **风险**：联网查询的失败处理（离线/registry 挂了）需设计；不要把「无法查最新版」搞成阻断错误。

### 选项 C：其他（明天可提出）

比如：
- `:check-agent` 加 `--upgrade` flag，默认只体检、显式才升级（避免每次体检都联网装）。
- 不动 `:check-agent`，另做一个 `:upgrade-agent` 命令，职责分离。

### 附：D20 改名的独立处理

无论选 A 还是 B，D20 改名这次的 receiver 更新都得有个交代：
- 选 A：走 npm `1.0.2` + 手动 update。
- 选 B：等 B 实现完，`:check-agent` 自动拉 `1.0.2`；或 B 实现前先手动一次。

> 倾向性（待确认）：**B**，但先 **A** 救急（D20 改名先发 1.0.2 手动更新），B 作为后续增强。理由：B 的价值是长期的（覆盖所有未来实现变更），不该和单次改名绑死；但 B 需要设计+测试，今晚来不及。

---

## 五、开放问题清单

1. **B 的联网查询失败怎么处理**？静默跳过（只体检不升级）还是报 warning？
2. **B 要不要默认开启**？还是做成 opt-in（配置项 / flag），避免用户每次 `:check-agent` 都被动升级？
3. **opencode（源码加载）的「版本跟进」怎么办**？它没有 npm version。git pull？还是承认源码加载的 agent 本就该用户自己 `git pull`，不归 `:check-agent` 管？
4. **升级后是否提示用户重启 agent**？receiver 重启才会重新注册到新目录 —— 这一步 `:check-agent` 替不了。
5. **这次 D20 改名，协议号到底动不动**？倾向**不动**（保持 `aibp-1`），改名是实现层变更。

---

## 六、相关文档与代码

| 项 | 位置 |
|----|------|
| 改名任务 | `D20-aibp-regdir-rename.md` |
| 协议版本语义 | `说明-AIBP.md §7`（尤其 §7.1） |
| `:check-agent` 入口 | `internal/action/command_neo.go:CheckAibpCmd` |
| Ensure 编排（核心） | `internal/aibp/ensure_agents/ensure.go:47` |
| pi 的版本/安装实现 | `internal/aibp/ensure_agents/ensure_pi.go`（`AIBPVersion`:80, `InstallAIBP`:98） |
| opencode 的版本/安装实现 | `internal/aibp/ensure_agents/ensure_opencode.go`（`AIBPVersion`:98, `InstallAIBP`:121） |
| pi 加载方式实证 | `~/.pi/agent/settings.json`、`~/.pi/agent/npm/node_modules/aibp-pi/` |

---

## 七、明天讨论的切入点建议

1. 先确认 **D20 改名这次怎么收尾**（A 救急）—— 这个有明确答案，快速过。
2. 再讨论 **要不要做 B**（长期机制）—— 这是开放设计，重点花时间。
3. B 若决定做，**当场起 D24 实施文档**，定接口、定联网策略、定默认行为。
