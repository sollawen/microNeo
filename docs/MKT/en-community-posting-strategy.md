# microNeo 英文社区发帖策略（HN / Reddit）

> **Purpose**: microNeo 冷启动阶段，如何在 Hacker News 和 Reddit 上发帖、不踩坑、拿到精准流量。
> **Context**: 作者账号 karma 少，曾「一说话就被删」。本文分析两个平台的真实规则差异，给出可执行的发帖方案。
> **Owner**: sollawen
> **Date**: 2026-06-29

---

## 0. 背景与核心结论

### 0.1 现状

- 知乎贴子（6-29 发布）约 1000 阅读，下载转化约 10 次（端到端 ~1%）
- 作者在 HN / Reddit 的账号 karma 很少，曾发帖被删
- 结论：**不能硬刚**，要绕开 karma 墙，用对平台、对姿势

### 0.2 核心判断

| 平台 | 新号能否发帖 | 被删的真实原因 | 难度 |
|------|------------|--------------|------|
| **HN** | ✅ **无 karma 门槛** | 不是 karma 问题，是格式 / 内容像广告 | 🟢 低 |
| **Reddit** | ❌ 各 sub 自己定 karma 门槛 | 真的被 mod 自动删 | 🔴 高 |

**最重要的一句话**：HN 没有 karma 限制，新号完全可以发。之前被删大概率是姿势不对（self-promo 触发了 anti-spam），不是账号问题。

---

## 1. 目标用户画像（为什么选这两个平台）

microNeo 的真实用户经过三层筛选：

```
会用终端编辑器
  ↓ 筛掉 99% 的人
会用 AI agent（pi / opencode / claude cli）
  ↓ 又筛掉一大批
= 职业开发者 + 日常 Linux/Mac + 已配 VPN + 有 GitHub 习惯
```

对这群人：
- VPN 不是门槛（工作刚需）→ 不需要做国内镜像
- 他们在 HN / Reddit 上**主动发现新工具**，是高意图流量
- 比知乎的泛开发者流量精准 10 倍以上

---

## 2. Hacker News 策略

### 2.1 平台规则澄清

- **HN 不限制新号发帖 / 评论**，不卡 karma
- HN 有 anti-spam 自动 flag 系统，会针对「新号 + 外链 + 推广话术」触发
- **破解方法**：用官方鼓励的 `Show HN` 格式发帖，有专门的宽容度

### 2.2 Show HN 格式（最推荐）

**官方定义**：Show HN 是 HN 专门给作者自荐项目用的板块，规则要求「讲清楚做了什么、为什么做、怎么做的」，不能纯营销。

**标题写法**（以 `Show HN:` 开头）：

| ❌ 差（太像广告） | ✅ 好（技术钩子 + 具体交互） |
|----------------|------------------------|
| `microNeo - terminal markdown editor with AI integration` | `Show HN: I made a terminal editor that talks to AI agents (alt-enter to send)` |

好标题的四要素：
1. `Show HN:` 前缀（格式合规）
2. 第一人称「I made」（人格化，不是产品官话）
3. 技术钩子（terminal + AI agents）
4. 具体交互细节（alt-enter to send）—— 让人好奇想点

### 2.3 正文模板（英文）

```
I've been doing "vibe coding" — writing less code by hand, spending more time
discussing plans with AI agents. The pain point: I always need to tell the AI
exactly which lines of a doc I have thoughts about, which means constant
copy-pasting between editor and chat.

So I built microNeo — a terminal editor that can send your selected text to
AI agents (pi / opencode, claude cli in progress).

How it works:
- Open a markdown / code file
- Select the text you want to comment on
- alt-enter → opens an input box
- alt-enter again → sends to the AI's current conversation

Some technical details that might interest HN:
- Forked from micro (zero-dep Go terminal editor), ~13MB single binary
- Markdown rendered in-place (tables, code blocks, headings) — not raw markup
- AIBP protocol for editor → agent IPC (registry-based, multi-receiver)

Would love feedback, especially on:
- the editor ↔ agent IPC design
- whether the markdown rendering approach makes sense
- what other agents you'd want supported

Repo: https://github.com/sollawen/microNeo
Demo video: <youtube or mp4 link>
```

**为什么这样写有效**：
- 开头讲**痛点故事**（HN 最爱），不是产品功能列表
- 中间有**技术细节**（IPC protocol、zero-dep、markdown 渲染方案）—— HN 用户吃这套
- 结尾**主动求 feedback**，不是求 star —— 姿态低、引发讨论
- 没有营销词（不用 "powerful" / "seamless" / "revolutionary"）

### 2.4 发帖时机

- **美西时间周二 / 三 / 四 早上 8–10 点**（北京晚上 11 点–凌晨 1 点）
- 周末和周一流量低，周五容易沉
- 避开重大新闻日（苹果发布会、大公司财报），否则你的帖会被挤下去

### 2.5 评论维护

发完 2 小时内**必须盯着评论**：
- 每条评论都认真回（HN 文化，不回 = 没诚意，帖会沉）
- 回复要**技术性、有细节**，不要只说 "thanks"
- 遇到批评**虚心接受**，别辩论 —— HN 最反感作者护短
- 有 bug 当场记下来承诺修，这种态度会涨粉

---

## 3. Reddit 策略

### 3.1 平台规则（真的卡 karma）

- 各 sub 自己设 karma 门槛（通常要求 50–100 comment karma 才能发帖）
- 新号直接发项目链接 → **大概率被 mod 删 + 可能 ban**
- Reddit 官方有 9:1 原则：每发 1 次自己内容，要先贡献 9 次别人讨论

### 3.2 推荐的 sub

| Sub | 用户画像 | 适配度 | karma 要求 |
|-----|---------|-------|----------|
| r/commandline | CLI 工具爱好者 | ⭐⭐⭐⭐⭐ | 中 |
| r/programming | 泛开发者 | ⭐⭐⭐ | 高 |
| r/vim / r/neovim | 编辑器用户（会拿 micro/helix 比） | ⭐⭐⭐⭐ | 中 |
| r/MachineLearning | 太学术，不推荐 | ⭐ | — |
| r/LocalLLaMA | 本地 AI 用户（可能用 opencode） | ⭐⭐⭐ | 中 |

### 3.3 养号流程（2 周计划）

**Week 1：纯贡献，不提项目**
- 每天 15 分钟，在 r/commandline / r/vim 真诚评论别人帖子
- 回答问题（vim 配置、shell 脚本、终端工具对比）
- 提问也行（"how do you handle X with Y tool?"）
- 目标：攒 30–50 comment karma

**Week 2：继续贡献 + 准备发帖**
- 持续评论，目标到 50–100 karma
- 准备好帖子的两种版本（见下）

**Week 3：发帖**
- 选 r/commandline 先试（门槛相对低）
- 帖子用「分享经验」姿势，不是「看我项目」

### 3.4 帖子写法（关键差异）

**❌ 错误（会被删）**：
```
Title: Check out my new terminal editor microNeo!
Body: I built microNeo, it has X Y Z features. Repo: ...
```

**✅ 正确（分享经验姿势）**：
```
Title: I forked micro to make a terminal editor that talks to AI agents — sharing what I learned

Body:
Been doing vibe coding for a while and got tired of copy-pasting between
my editor and AI chat. So I forked micro (the Go terminal editor) and
added a way to send selected text directly to AI agents.

Some things I learned building this:

1. Editor ↔ agent IPC is harder than it looks. Ended up with a
   registry-based protocol (aibp-2.0) so multiple agents can coexist.
   Happy to go into details if anyone's curious.

2. Markdown rendering in terminal is a rabbit hole. Tables, code blocks,
   headings — each has its own rendering pipeline. Took ~3 iterations
   to get cursor scrolling right across multi-line segments.

3. The actual UX win is tiny: alt-enter to send, instead of copy-paste.
   But it changed my workflow more than I expected.

What I'm still unsure about:
- Should this be a micro plugin instead of a fork? Went with fork for
  deeper integration but not sure.
- Which other AI agents should I support? (currently pi + opencode)

Repo if anyone wants to poke: https://github.com/sollawen/microNeo
```

**为什么这样写有效**：
- 标题强调「I forked」+「sharing what I learned」—— 分享者姿态，不是推广者
- 正文大量**技术教训**（IPC、markdown 渲染、cursor scrolling）—— Reddit 程序员吃这套
- 主动暴露**不确定**（"not sure", "should I"）—— 引发讨论而不是辩论
- Repo 链接放最后，弱化推广感

---

## 4. 不推荐硬刚的方案

| 方案 | 为什么不推荐 |
|------|------------|
| 新号直接发项目链接 | 被 mod 删 + 可能 ban，浪费账号 |
| 买老号 | Reddit 检测异常登录，可能 ban；违反 ToS |
| 找朋友刷 karma | 同上，且破坏社区信任 |

---

## 5. 更高效的破局路径（绕开 karma 墙）

既然 HN / Reddit 有门槛，**先做容易的**：

| 方案 | 难度 | 用户精准度 | 备注 |
|------|------|----------|------|
| **opencode / pi 的 Discord 社群** | 🟢 低 | ⭐⭐⭐⭐⭐ | 用户就是目标人群，对生态工具欢迎 |
| **HN Show HN** | 🟢 低 | ⭐⭐⭐⭐ | 不需要 karma，用对格式即可 |
| **找有 karma 的朋友代发 Reddit** | 🟡 中 | ⭐⭐⭐⭐ | 借力，最快 |
| Reddit 自己养号 | 🔴 高 | ⭐⭐⭐⭐ | 耗时 2 周，但长期资产 |

**最优执行顺序**：
1. **本周**：opencode / pi Discord 发（最快出效果，用户最精准）
2. **本周**：HN Show HN（用 §2.3 的模板，时机选美西早 9 点）
3. **下周**：Reddit 养号开始（按 §3.3 的 2 周计划）
4. **2 周后**：Reddit r/commandline 发帖

---

## 6. 可复用的内容素材

### 6.1 一句话定位（电梯演讲）

> A terminal editor that sends your selected text to AI agents — stop copy-pasting between editor and chat.

### 6.2 核心卖点（按优先级）

1. **alt-enter 发送选中文字给 AI**（最核心差异点，必须突出）
2. **Markdown 原地渲染**（读 AI 写的文档舒服）
3. **13MB 单文件、零依赖**（程序员最爱的"不污染系统"）
4. **支持多 AI agent**（pi / opencode，claude cli 开发中）
5. **Fork 自 micro，保留全部编辑能力**（不是阉割版）

### 6.3 演示素材

- `assets/aibp-opencode.mp4`（网站首页用的视频，最直观）
- 建议额外录一个 **30 秒短视频**（select → alt-enter → AI 收到的完整流程），适合 Reddit / Twitter

---

## 7. 衡量指标（发帖后追踪）

| 指标 | 怎么测 | 期望值 |
|------|-------|-------|
| HN 帖子 points | 帖子页面 | 10–30 算成功（破 50 算爆款） |
| HN 评论数 | 帖子页面 | 5–15 算有讨论度 |
| GitHub stars 当日增量 | `gh api .../traffic` | +20–50 算有效 |
| GitHub 当日独立访客 | `gh api .../traffic/views` | 100+ 算有效引流 |
| Binary 下载当日增量 | release assets | +5–15 算有效 |

**冷启动现实预期**：
- 一次成功的 Show HN → 当日 +30~100 stars，长期带 200–500 访客
- 一次成功的 r/commandline → 当日 +10~30 stars
- 知乎 1000 阅读只带来 ~10 下载，HN/Reddit 1000 阅读能带来 30–80 下载（用户精准 3–8 倍）

---

## 8. 待办（明天 review）

- [ ] 把知乎贴子的痛点故事，改写成 §2.3 的 HN 模板
- [ ] 录一个 30 秒的 select → alt-enter 演示视频
- [ ] 找 opencode / pi 的 Discord 链接，准备社群文案
- [ ] 选 HN 发帖时机（美西周二 / 三早 9 点 = 北京周三 / 四凌晨 1 点）
- [ ] 决定是否启动 Reddit 养号（2 周计划）
