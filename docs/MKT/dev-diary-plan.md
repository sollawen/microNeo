# microNeo 开发日记系列 — 系统计划

> Last updated: 2026-06-19
> 状态：📋 计划中（第一篇未发布）
> 配套主计划：`MKT-plan.md`（日记是 "无门槛渠道" 内容源 + karma 弹药库）

---

## 1. 为什么开这个日记系列

不是"记录生活"，是**用一篇篇可独立成立的技术文章，同时解三个卡点**：

| 卡点（来自 MKT-plan） | dev diary 怎么解 |
|---|---|
| HN/Reddit 发不出主帖 | 每篇 = karma 弹药：HN/Reddit 上 "how I solved X" 类评论是高质量评论的现成模板 |
| dev.to / V2EX / 掘金 缺内容源 | diary 是天然长文素材库，不用每篇从零憋通稿 |
| v2.0 launch 缺叙事弧 | MKT-plan 定的 v2 故事是 "6 周前我发了 v1，学到 X"——diary 是提前 6 周开始铺这条 arc，到 v2 launch 时直接引用 |

**衡量标准（不是"写了多少篇"）：**
1. 发布篇数（shipped，不是 draft）
2. 每篇带来的 visitor（referral 链接，看 GitHub referrer）
3. 复用为 HN/Reddit 评论的次数（→ karma 增量）

---

## 2. 节奏与防拖延机制

**目标节奏：每 3-5 天一篇，不日更。**

理由：日更的隐性成本是每篇都变 filler，第二周读者退订、自己也写不下去。绝大多数 dev diary 死在第 4-5 篇。

**防拖延（这是过去 11 天的真问题）：**
- 6/9 定的 HN+Reddit 发帖 → 失败
- 6/19 定的 7 天冲刺（dev.to Day 1 / Twitter Day 1-2 / V2EX Day 2-3）→ **大部分还没 shipped**

> **历史诊断：瓶颈不是策略不够好，是 shipped 太少。** dev diary 必须自带防拖延，否则只是新的"我在做营销"错觉。

**机制：**
- 每篇给 **48 小时 hard deadline**：草稿开始算，48h 内必须发到至少一个渠道（dev.to 优先）
- 允许"小发布"：没写完可以发一个 shorter 版本到 V2EX，但**必须发**
- 公开承诺：第一篇发布时，在结尾写 "This is post #1 of a series. Next: …"——制造下一篇的压力

---

## 3. 质量门槛（什么 diary 会失败）

**失败模板（避免）：**
- ❌ "今天我做了 X" — 没人关心陌生人的日程
- ❌ 全篇吹产品 — 等于广告，HN 一秒折叠
- ❌ 纯流水账，没有 takeaway — 读者看完学不到东西

**成功模板（强制）：**
- ✅ **一个问题 → 我怎么解的 → microNeo 是案例**（问题在前，产品在后）
- ✅ **任何一篇，一个不知道 microNeo 的人读到也有收获**
- ✅ **有一个能记住的具体点**（"光标跨段滚动"、"双写不一致"——这种词）
- ✅ **诚实写失败**（"v1.0.5 我用了影子数组，结果..."）—— HN 最吃这个

**自检三问（发前必过）：**
1. 删掉所有 microNeo 名字，这篇文章还有价值吗？（应该有）
2. 一个 Go/terminal 中级开发者读完，能学到什么具体东西？
3. 有没有一句话别人会想 quote / 转发？

---

## 4. 选题池（按真实文档映射，不空想）

每个选题都绑定项目里的真实素材，避免"无米下锅"。

### A. 概念/痛点向（受众最广，适合发 HN/dev.to 当 hook）

| # | 题目（工作标题） | 核心点 | 素材来源 |
|---|---|---|---|
| A1 | 为什么终端里做 WYSIWYG Markdown 这么难 | 屏幕窄、ASCII、cursor 语义、渲染/编辑态切换——所有 GUI 不是问题的事 | `introduce.md`, `micro现状研究及我的想法.md`, `PRD-产品需求文档.md` |
| A2 | 为什么所有 Markdown 编辑器都在分屏，以及我为什么拒绝 | 痛点框架：分屏是 lazy design，terminal 不能更窄 | `introduce.md`, 竞品表（MKT-plan 附录 A） |

### B. 架构决策向（Go 社区、工程师吃这套）

| # | 题目 | 核心点 | 素材来源 |
|---|---|---|---|
| B1 | Fork 一个 25k-star 编辑器，怎么让代码不腐烂 | `*_md.go` 隔离策略、"对原生代码侵入越小越好" 原则、clean starting point 决策 | `架构设计V1.md`, `AGENTS.md` |
| B2 | Detect 和 Render 分离：让 700 行渲染管线不卡 | 事件驱动 detect、与屏宽无关、为什么不能每帧重算 | `架构设计V1.md`, `光标滚动方案B-实施总结.md` §2.1 |
| B3 | 单一数据源（single source of truth）在 TUI 渲染里怎么落地 | screenBuffer 架构、为什么"画两遍"是万恶之源 | `光标滚动方案B-实施总结.md` §1-2 |

### C. 硬核调试/重写向（HN narrative arc 最爱）

| # | 题目 | 核心点 | 素材来源 |
|---|---|---|---|
| C1 | 我把渲染架构推倒重写了——因为光标跨段滚动做不对 | v1.0.5 viewportRowmap 影子数组 → v1.0.6 screenBuffer，"双写不一致"根因 | `光标滚动方案B-实施总结.md` |
| C2 | 一个潜伏 4 个版本的多光标 Bug，和它教我的"分层存储" | buffer 层 vs 显示层，多光标信息在两步之间丢失 | `多光标显示Bug修复方案.md` |
| C3 | 装饰行向下映射、case 边界左闭右闭——一个 Bug 修了三次才对 | case A/B/C 边界条件、可见视口判定 | `光标滚动方案B-实施总结.md` §时间线 |

### D. 元向（适合 V2EX/掘金中文圈，个人叙事）

| # | 题目 | 核心点 | 素材来源 |
|---|---|---|---|
| D1 | 独立开发者第 N 周：我是怎么把一个 fork 做成产品的 | 时间线、版本演进、踩坑节奏 | release-notes-pipeline.md + 各版本 changelog |
| D2 | 我给 micro 写了 Markdown 渲染——文档先行，代码后写 | docs 目录的工程价值、PRD/架构/方案文档的用法 | docs/ 目录本身 |

---

## 5. 单篇发布 SOP

**每篇发布必须走完这三步（不满足不算 shipped）：**

| 步骤 | 渠道 | 形式 | 目的 |
|---|---|---|---|
| ① 主发 | dev.to（英文）或 掘金（中文） | 长文，1500-3000 字 | SEO 长尾，visitor 池主入口 |
| ② 拆解 | Twitter/X thread | 6-8 推，每推一句 + 一张图/代码块 | 病毒传播种子 |
| ③ 复用 | HN / Reddit 相关讨论里**自然引用**（不是刷链接） | "I wrote a deeper dive on this exact problem: [link]" | 攒 karma + 反向流量 |

**中文圈额外（D 类选题）：** V2EX 分享创造节点、掘金。

**禁忌：**
- 不要在 HN/Reddit 新号期主帖里贴自己项目链接（shadowban 风险，见 MKT-plan §Karma 手册）
- 不要每篇都喊 "please star"——结尾一句 "microNeo is open source, link in bio" 就够

**每篇固定结构（模板）：**
```
[Hook：一句话问题，不提产品]
↓
[为什么这个问题难——背景知识，让外行也能跟]
↓
[我的第一次尝试，和它为什么失败]  ← 诚实写
↓
[第二次/最终方案，核心技术]
↓
[一个具体的代码/图，证明不是空谈]
↓
[Takeaway：读者能带走什么]
↓
[一句软链接：microNeo 是这个故事的案例，open source]
```

---

## 6. 第一批 5 篇（提议，待讨论）

按"受众广度递减、技术深度递增"排序，让读者随系列逐步深入：

| 顺序 | 选题 | 一句话 hook | 主要渠道 |
|---|---|---|---|
| 1 | **A1：为什么终端 WYSIWYG MD 这么难** | "Every Markdown editor splits your screen. I spent 6 weeks figuring out why it's actually hard not to." | dev.to + Twitter |
| 2 | **B1：Fork 25k-star 编辑器怎么不腐烂** | "I forked Micro (25k★). The first decision wasn't features — it was how to keep the code mergeable upstream." | dev.to + HN 评论 |
| 3 | **C1：我推倒重写了渲染架构** | "v1.0.5 had a shadow array. It broke on cursors crossing table boundaries. v1.0.6 rewrote the whole render pipeline." | dev.to + Twitter |
| 4 | **C2：潜伏 4 版本的多光标 Bug** | "Multi-cursor worked for editing, but only showed one cursor on screen. The bug lived through 4 releases." | dev.to + Reddit r/programming |
| 5 | **B3：单一数据源在 TUI 里怎么落地** | "I was drawing the screen twice. Once to the terminal, once to a shadow buffer. They kept disagreeing." | dev.to + HN 评论 |

> 这 5 篇发完，正好覆盖：① 痛点叙事 ② 架构哲学 ③ 大重构故事 ④ 微妙 Bug 故事 ⑤ 设计原则。每个都是独立 hook，但串起来就是完整的"我做了 microNeo，这是我的工程哲学" arc——v2.0 launch 时直接引用。

---

## 7. 跟踪表（每篇发布后更新）

| # | 选题 | 状态 | dev.to | Twitter | V2EX/掘金 | HN/Reddit 评论引用 | 该篇带来 visitor | 备注 |
|---|---|---|---|---|---|---|---|---|
| 1 | A1 终端 WYSIWYG 之难 | 📋 待写 | — | — | — | — | — | — |
| 2 | B1 Fork 不腐烂 | 📋 待写 | — | — | — | — | — | — |
| 3 | C1 重写渲染架构 | 📋 待写 | — | — | — | — | — | — |
| 4 | C2 多光标 Bug | 📋 待写 | — | — | — | — | — | — |
| 5 | B3 单一数据源 | 📋 待写 | — | — | — | — | — | — |

---

## 8. 风险与退出条件

**什么时候该停掉这个系列：**
- 连续 3 篇发布后 0 visitor、0 评论引用 → 渠道选错或质量不够，**先复盘再写下一篇**，不要硬写
- 自己写到第 5 篇已经心累、每篇凑字数 → 降频到每 2 周一篇，或转"只在有真东西时写"

**什么时候该加码：**
- 某篇带来 ≥20 visitor 或被 HN/Reddit 评论引用 ≥3 次 → 这个选题有共振，立刻写同主题续篇
- 总 visitor 池从 34 拉到 200+ → 日记策略验证成功，可在 v2 launch 时打包成 "engineering blog" 链接放 README

---

## 9. 与 MKT-plan 的衔接

- **本计划 = MKT-plan "立即行动 7 天冲刺" 的内容生产侧**。MKT-plan 负责"发到哪"，本计划负责"写什么、怎么写、什么节奏"。
- 第一篇发布后，回填 MKT-plan §执行进度，并把本计划 §7 跟踪表的真实数据同步到 MKT-plan §数据快照。
- v2.0 launch 前 1 周：把本系列前 N 篇整理成 "Engineering notes" 列表，作为 Show HN 主帖的 "more background" 链接。
