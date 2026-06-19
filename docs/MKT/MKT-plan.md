# microNeo Marketing Plan

> Last updated: 2026-06-19（SEO 重大优化）

---

## 🔍 SEO 重大优化（2026-06-19 完成）🆕

**一次完成 7 项 Google 搜索优化改动**，从内容到 metadata 全链路覆盖。

| # | 改动 | 位置 | 关键变化 |
|---|---|---|---|
| 1 | **FAQ 加 8 个 Q&A** | README 第 41 行 | 抢 PAA / Featured Snippet，H3 自然语言问句 |
| 2 | **Headline callout "Also a full text editor"** | README 顶部 | 拓展定位，吸引非 MD 用户 |
| 3 | **Q5 (vs Micro) 强化** | README FAQ | 加 "keeps all of that"，防止用户以为丢能力 |
| 4 | **新 Q8 "Can I edit code/config?"** | README FAQ | 抢 "code editor terminal" / "syntax highlighting" 长尾 |
| 5 | **demo 图加 alt 文本** | README 第 14 行 | Google 图片搜索流量 |
| 6 | **Topics 优化：8 → 12 个** | GitHub metadata | 去掉 `markdown`，加 `markdown-editor` 等 5 个 |
| 7 | **Description 重写：54 → 180 字符** | GitHub metadata | 全部关键词覆盖，Google 搜索摘要直接可用 |

**预期效果时间线**：
- 📅 1-3 天：Google 重新爬 README + 索引新 topics
- 📅 1-2 周：FAQ Q&A 命中 PAA / Featured Snippet
- 📅 2-4 周：organic 搜索流量开始显现

**关键判断**：
- 之前流量 100% 依赖 awesome-tuis 长尾 + 同事口碑
- 这次 SEO 改动让 microNeo 在搜索结果里 **self-serve** —— 用户搜 "terminal markdown editor" / "syntax highlighting terminal" / "Claude Code editor" 都能找到
- 配合 v1.0.x release news，**awesome-tuis 长尾 + 搜索流量 = 双引擎**

**详见附录 C：SEO 关键词覆盖矩阵**。

---

## 📣 状态更新（2026-06-19）

**最近里程碑：**
- ✅ **awesome-tuis PR #713 已 merge**（[链接](https://github.com/rothgar/awesome-tuis/pull/713)）—— 第一个英文 awesome 列表收录（19.3k ⭐，由 Justin Garrison 合并）
  - README badge 已加：`https://github.com/rothgar/awesome-tuis`
  - 当前流量贡献：14 天 96 次站内 referrer，awesome-tuis 应是主长尾来源
  - 预期长期效果：每天 10-50 浏览，持续带来 organic stars
- ✅ **chinese-independent-developer PR #975 已 merge**（[链接](https://github.com/1c7/chinese-independent-developer/pull/975)）—— 48.8k ⭐，6/9 合并
  - 6-08 流量高峰（45 views / 79 clones uniques）由这次 merge 触发

**PR 状态（6/19 全面盘查）：**
- ✅ rothgar/awesome-tuis #713（19.3k）— MERGED 6/16
- ✅ 1c7/chinese-independent-developer #975（48.8k）— MERGED 6/9
- ❌ **agarrharr/awesome-cli-apps #1142（19.7k）— CLOSED 6/8（6 小时内被 jneidel 关掉，无评论）** — 见下方"重提策略"
- 🟡 thegdsks/awesome-modern-cli #20（368）— OPEN 12 天
- 🟡 BubuAnabelas/awesome-markdown #124（933）— OPEN 12 天
- 🟡 521xueweihan/HelloGitHub #3335（160k）— Issue 投稿，月刊审核

**详见下方"🆕 新目标待提交"和"附录 B：awesome PR 完整记录"。**

---

## ⚠️ 渠道卡点现状（2026-06-19 更新）

**6/9 计划发的 HN + Reddit 全部失败，卡在账号门槛：**

| 渠道 | 卡点 | 状态 |
|---|---|---|
| HN Show HN | 新号无 karma，无法发链接 | ❌ blocked |
| Reddit r/commandline | 当前 8 karma，sub 要求 ≥50 | ❌ blocked |
| Reddit r/Markdown | 同上 | ❌ 大概率被卡 |

**策略调整：**
- 主战场从 HN/Reddit 转向 **无门槛渠道**：dev.to / Twitter / V2EX / 掘金
- HN/Reddit 降级为 **后台 karma 攒分任务**（2-3 周解锁）
- 解锁后 Show HN / Reddit 主帖补发，配合下个 release 版本

**为什么不在 install.sh 上做文章：**
- install.sh 走 binary 下载，不走 `git clone`，GitHub 看不到这部分调用
- binary 下载数 ≈ 真实装机数，是 install.sh 的代理指标（**当前 ~20/7d 或 ~41/终身**）
- 改成 `git clone` 会把装机数砍 5-10×，得不偿失
- 所有成功的 CLI 工具（bat/fd/ripgrep/zoxide）都走 `curl | sh`

---

## 📊 数据快照（2026-06-19 GitHub API 抓取）

| 指标 | 值 | 说明 |
|---|---|---|
| 总 stars | 10 | 6 来自同事，**organic = 4** |
| 14 天 unique visitor | 45 | 真的点进 GitHub 看 README/demo 的人 |
| 14 天 git clone unique | 441 | ⚠️ **大部分是 dev/CI/bot，非真用户** |
| **⭐ 7d binary 下载** | **20** | ⭐ **真用户装机数**（基于 GitHub Releases API） |
| **⭐ 终身 binary 下载** | **41** | ⭐ **真用户装机累计** |
| **⭐ visitor → install 转化率** | **~15%** | 41 安装 / 271 浏览 = 远高于行业 2-5% |
| 全部 release binary 总下载 | 41 | M1+ Mac 占 49%，Linux x64 占 37% |
| Top referrer | github.com | awesome-tuis 长尾主入口 |

**关键洞察：**
1. 12% 转化率证明 **产品定位 + README + demo 没问题**
2. 真正瓶颈是 **discovery 入口太少**（visitor 池太小）
3. install.sh 不构成卡点，不要动
4. 唯一出路：**扩大 discovery 入口** → 解锁后才有更多 visitor → 维持 12% 转化

---

## 🎯 营销节奏战略（2026-06-19 定稿）

**核心判断：不等到 v2.0 才宣传。v1.x 内容是 v2.0 launch 的前置燃料，不是浪费。**

### 为什么不能等到 v2.0

1. **karma 攒分是 v2.0 launch 的前置依赖**：HN 3-5 天解锁、Reddit 2-3 周解锁。不攒，v2.0 出来时主战场还是发不出
2. **无门槛渠道（dev.to / Twitter / V2EX / 掘金）跟 v2.0 正交**：现在发 v1 内容不会因为 v2 出来而贬值，反而能升级为 v2 预告
3. **"v1 → v2 演进" 比 "凭空 v2" 强**：HN/Reddit 喜欢 narrative arc（"v1 出来后我学到 X，现在做 v2"）
4. **12% 转化率证明 v1 核心 OK**：缺的是 visitor 池（34 → 需要更大），不是产品吸引力

### 营销时间线

| 时间窗 | 主战场动作 | 后台 | 目的 |
|--------|--------|------|------|
| **现在 → +2 周** | dev.to 长文（v1 故事）+ Twitter thread + V2EX/掘金 | HN/Reddit karma 攒分 | 解锁主战场 + 建 audience |
| **+2 周 → v2.0** | v1.x 维护 + 偶尔 Twitter 更新 + 中文圈持续 | 继续攒分 | 保持 momentum + 升级 v2 hook |
| **v2.0 release 前 1 周** | README 加 "v2.0 coming soon" 区块 + dev.to 预告 + 中文圈预热 | — | 蓄势 |
| **v2.0 release 当天** | Show HN + Reddit r/commandline + r/Markdown + r/terminal 四连发 + Twitter 主 thread + 中文圈同步 | — | 集中引爆 |

### v2.0 launch 的故事弧（已定稿，HN/Reddit 通用）

```
Six weeks ago I shipped v1 — a terminal Markdown editor that 
renders and edits in the same window. It got X stars and Y feedback. 
Here's what I learned.

Today v2 ships: it now talks to AI agents too, via an LSP-style 
protocol. Add a new agent without touching microNeo's code.
```

### v2.0 核心卖点（agent-comm，见 dev2 分支 `docs/agent-comm/`）

- **LSP 式 AI agent bridge**：microNeo 定义协议，新 agent 接不用改 microNeo 任何代码
- **2026 年真痛点**：在 terminal 编辑器里直接控制 AI agent
- **跨 agent 生态**：pi、opencode 等多 agent 可接入
- **完整架构文档**：dev2 分支 `docs/agent-comm/README.md` 是总纲

### 风险对冲

- **如果 v2.0 延期**：v1 内容仍持续有效，不浪费
- **如果 v2.0 提前**：karma 还没攒够就是大问题 —— 所以现在就开始攒
- **关键判断**：v1.0 **不是**"还不够吸引人"，是 visitor 池太小；v1 声量 + v2 故事 = 主战场 launch 时的双重燃料

---

## 核心卖点（所有渠道统一）

**一句话：**
> The only terminal Markdown editor that renders and edits in the same window.

**扩展版：**
> Every Markdown editor splits your screen — source left, preview right.
> Terminal screens aren't wide to begin with. Splitting makes it worse.
> microNeo renders and edits in the same window. Click to edit. See the result instantly.
> Nothing else does this.

---

## 🎉 里程碑

| 日期 | 事件 | 意义 |
|------|------|------|
| 2026-06-09 | chinese-independent-developer PR #975 | ✅ 已 merge（48.8k ⭐ 中文开发者社区收录） |
| 2026-06-16 | **awesome-tuis PR #713 merge** | ✅ 已 merge（19.3k ⭐，由 Justin Garrison 合并）。**第一个英文 awesome 列表收录** |

---

## 执行进度

### ✅ Day 1 — 2026-06-08（周日）

| 任务 | 状态 | 链接/说明 |
|------|------|-----------|
| 竞品调研（HN 数据、GitHub stars） | ✅ 完成 | 见附录 A |
| 确定核心卖点：同窗口渲染 + 编辑 | ✅ 完成 | 全网无竞品，验证过 |
| awesome-tuis PR | ✅ **已 merge** | [PR #713](https://github.com/rothgar/awesome-tuis/pull/713)（19.3k ⭐，6/16 由 Justin Garrison merge） |
| awesome-cli-apps PR | ✅ 已提交 | [PR #1142](https://github.com/agarrharr/awesome-cli-apps/pull/1142)（19.7k ⭐） |
| awesome-modern-cli PR | ✅ 已提交 | [PR #20](https://github.com/thegdsks/awesome-modern-cli/pull/20)（361 ⭐） |
| awesome-markdown PR | ✅ 已提交 | [PR #124](https://github.com/BubuAnabelas/awesome-markdown/pull/124)（931 ⭐） |
| 中国独立开发者列表 PR | ✅ 已提交 | [PR #975](https://github.com/1c7/chinese-independent-developer/pull/975)（48.8k ⭐ 程序员版面） |
| HelloGitHub 投稿 | ✅ 已提交 | [Issue #3335](https://github.com/521xueweihan/HelloGitHub/issues/3335)（160k ⭐ 月刊） |
| 安装 gh (GitHub CLI) | ✅ 完成 | `gh auth login` as sollawen |
| 推广调研报告 | ✅ 完成 | `docs/推广调研报告.md` |

### ✅ Day 2 — 2026-06-09（周一）白天准备

| 任务 | 优先级 | 状态 |
|------|--------|------|
| **优化 README** | P0 | ✅ 定稿，去掉 AI era，压缩 Why microNeo，截图提前 |
| **检查 demo 截图** | P0 | ✅ 确认 microneo-demo2.png 效果 OK |
| **起草推广通稿** | P0 | ✅ HN + Reddit 通用稿件已写好，见下方通稿章节 |
| **GitHub Release** | P0 | ✅ 已有 v1.0.2，6 平台二进制 + SHA |

### ✅ Day 3 — 2026-06-19（周四）SEO 重大优化

| 任务 | 状态 | 链接/说明 |
|------|------|-----------|
| **FAQ 8 个 Q&A 写入 README** | ✅ 完成 | 第 41 行，原占位 `## FAQ` 填充。风格："一个段落 = 一行"，全章统一 |
| **Headline 加 "Also a full text editor"** | ✅ 完成 | 拓展定位，告知 100+ 语言、daily driver |
| **Q5 (vs Micro) 强化** | ✅ 完成 | 加 "keeps all of that and adds Markdown" |
| **新 Q8 (code/config files)** | ✅ 完成 | 抢 "code editor terminal" / "syntax highlighting" 长尾 |
| **demo 图加 alt 文本** | ✅ 完成 | README 第 14 行 |
| **Topics 优化** | ✅ 完成 | 8 → 12 个，详见下面 "当前 topics" |
| **Description 重写** | ✅ 完成 | 54 → 180 字符，添加 "renders and edits" / "syntax highlighting" / "Claude Code" 等关键词 |

**当前 topics（12 个）**：
```
editor, terminal, go, tui, micro, cli, linux,           # 基本身份
claude-code, cross-platform,                            # 增长词
markdown-editor, markdown-rendering, syntax-highlighting  # 差异化
```

**Description 当前版本（180 字符）**：
> Terminal Markdown editor that renders and edits in the same window. Single Go binary with syntax highlighting for 100+ languages, works as $EDITOR for Claude Code and any CLI tool.

**为什么这次重要**：
- 之前流量 100% 依赖 awesome-tuis 长尾 + 同事口碑
- 现在加了 organic 搜索引擎：**self-serve discovery**
- 搜索词覆盖：品牌 / 父项目 / 竞品 / 用例 / 技术 / 痛点 / 平台全关键词

### 📋 Day 2 — 2026-06-09（周一）晚上发布（❌ 全部被卡）

| 任务 | 时间 | 实际结果 |
|------|------|----------|
| **HN Show HN 发帖** | 北京时间 20:00-21:00 | ❌ 新号无 karma，发链接失败 |
| **Reddit r/commandline** | 同上 | ❌ 8 karma 不达 r/commandline 门槛，被自动移除 |

### 🚀 立即行动（2026-06-19 起，7 天冲刺）

**目标：解锁 "无门槛渠道"，先把 visitor 池从 34 拉到 200+**

| # | 任务 | 时间 | 渠道 | 内容源 |
|---|------|------|------|--------|
| 1 | **dev.to 长文** | Day 1（今天） | dev.to | 现有通稿展开 + 技术故事（fork micro 动机、scroll/渲染难点） |
| 2 | **Twitter/X thread** | Day 1-2 | Twitter | 6-8 张 demo 图 + 一句话 hook，@ 几个 terminal KOL |
| 3 | **V2EX 分享创造** | Day 2-3 | v2ex.com | 通稿中文版，标题直白 |
| 4 | **掘金长文** | Day 3-5 | juejin.cn | 同 V2EX 但展开个人故事 |
| 5 | **跟 awesome-cli-apps PR #1142** | Day 2 | GitHub | 礼貌 ping 维护者，10 天了没动静 |

### 🔄 后台进行（不阻塞主路径）

| 任务 | 节奏 | 解锁时间 | 解锁后动作 |
|------|------|----------|-----------|
| **HN 攒 karma** | 每天 15 分钟高质量评论 | 3-5 天 | 解锁 Show HN → 发 v1.0.9 / v1.0.10 |
| **Reddit 攒 karma** | r/commandline / r/Markdown / r/terminal 每天 1-2 条实质评论 | 2-3 周到 50 | 解锁后发主帖（通稿去掉 "Show HN:"） |

### 🟢 后续（Week 2+）

| 任务 | 说明 |
|------|------|
| HN Show HN（解锁后） | 配合下个 release 做 "Update" 帖 |
| Reddit r/commandline（解锁后） | 同上 |
| dev.to 中文版（同步） | 跨语言长尾 |
| 知乎 / 即刻 | 中文长尾备份 |
| Homebrew formula | 暂缓，等 200+ stars 再尝试 homebrew-core |
| awesome 列表跟进 | 3 个 PR 持续关注；如未 merge 考虑 fork 二级列表 |

---

## 推广通稿（所有渠道共用）

> 发布前按平台规则微调，Last updated: 2026-06-09

### 正文

```
I use Micro to edit Markdown files, and Glow to read them. The constant switching drove me crazy.

Every other solution either can't edit (Glow, frogmouth) or splits your screen in two (vim plugins, GUI editors). Terminal screens aren't wide to begin with — splitting them is painful.

So I built microNeo: a standalone terminal Markdown editor that renders and edits in the same window. Open any .md file, you see it rendered. Click anywhere to edit the source. No split panes.

[截图]

Built in Go, single binary. One-line install:
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/install.sh | sh

Works great as $EDITOR for Claude Code, Yazi, etc.

https://github.com/sollawen/microNeo
```

### 对比表（回复评论用）

| | microNeo | Micro / nano | glow / leaf | vim + plugins | GUI Editors |
|--|:---------:|:-------------:|:------------:|:------------:|:-------------:|
| **Editable** | ✅ | ✅ | ❌ | ✅ | ✅ |
| **Markdown Rendering** | ✅ | ❌ | ✅ | ✅ | ✅ |
| **Same Interface** | ✅ | — | — | ❌ (split) | ❌ (split) |
| **Low Learning Curve** | ✅ | ✅ | ✅ | ❌ | ✅ |

### 各平台差异

| | HN | Reddit | V2EX |
|--|-----|--------|------|
| 标题 | Show HN: microNeo – A terminal Markdown editor that renders and edits in the same window | 同左（去掉 Show HN:） | 待定 |
| 语言 | English | English | 中文 |
| 正文长度 | ≤ 300 字 | 不限 | 不限 |
| 链接 | 正文末尾 | 正文末尾 | 正文末尾 |
| 态度 | 谦虚，接受批评 | 友好，诚实说是自己做的 | 分享创造，轻松 |

### HN 发帖规则提醒

- 标题以 `Show HN:` 开头
- 正文不能太长，300 字以内
- **全天回复评论**——HN 的评论互动直接影响排名
- 不要自己顶帖，不要叫朋友顶帖（HN 会检测并惩罚）
- 态度谦虚，接受批评

### 待起草（已纳入 7 天冲刺，见上方 "立即行动"）

- **dev.to 英文版** — Day 1（今天），技术故事展开
- **V2EX 中文版** — Day 2-3，分享创造节点
- **掘金长文** — Day 3-5，个人故事角度

---

## README 优化要点（Day 2 执行）

1. **去掉 "AI era"** — HN 用户反感 AI 炒作，用实际痛点切入
2. **压缩 Why microNeo** — 5 个小节 → 2-3 句话 + 对比表格
3. **首屏一句话说明** — 先说产品是什么，再讲为什么
4. **截图放最前面** — 视觉冲击力 > 文字
5. **不需要 GIF** — microNeo 是通用编辑器，核心卖点"同窗口渲染+编辑"静态截图就能传达，没有特别需要演示的操作

---

## 推广渠道优先级（2026-06-19 更新）

| 优先级 | 渠道 | 预期效果 | 状态 |
|--------|------|----------|------|
| 🔴 P0 | **GitHub organic 搜索** | **新 ✅**：self-serve discovery，靠 6/19 SEO 改动被动接收流量 | ✅ 7 项 SEO 改动完成 |
| 🔴 P0 | **dev.to 长文** | SEO 长尾 50-200 stars | 📋 Day 1（今天） |
| 🔴 P0 | **Twitter/X thread** | 病毒传播 0-500 stars | 📋 Day 1-2 |
| 🔴 P0 | **V2EX 分享创造** | 中文圈 50-200 stars | 📋 Day 2-3 |
| 🔴 P0 | **mundimark/awesome-markdown-editors** | 精准 fit 长期 | 🆕 待提交（6/19 立项） |
| 🔴 P0 | **mundimark/awesome-markdown** | 备份 list 长期 | 🆕 待提交（6/19 立项） |
| 🔴 P0 | **hackstoic/golang-open-source-projects** | 中文 Go 圈 | 🆕 待提交（6/19 立项） |
| 🔴 P0 | awesome-tuis | 长期每天 10-50 浏览 | ✅ 已 merge |
| 🟡 P1 | **掘金长文** | 中文长尾 100-500 浏览 | 📋 Day 3-5 |
| 🟡 P1 | HN 攒 karma | 解锁 Show HN | 🔄 后台 3-5 天 |
| 🟡 P1 | Reddit 攒 karma | 解锁 r/commandline | 🔄 后台 2-3 周 |
| 🟡 P1 | **herrbischoff/awesome-macos-command-line** | 30k 英文大列表 | ⏸️ 等 200+ stars |
| 🟡 P1 | awesome-modern-cli #20 | 月更，跟进 | 🟡 OPEN 12 天 |
| 🟡 P1 | awesome-markdown #124 | owner 僵尸风险 | 🟡 OPEN 12 天 |
| 🟡 P1 | HelloGitHub #3335 | 160k 月刊 | 🟡 等月刊 |
| 🟢 P2 | HN Show HN（解锁后） | 0-300 stars | 📋 解锁后 |
| 🟢 P2 | Reddit r/commandline（解锁后） | 50-200 浏览 | 📋 解锁后 |
| 🟢 P2 | **awesome-cli-apps 重提** | 200+ stars 后用极简 PR | ⏸️ 等 stars |
| 🟢 P2 | alebcay/awesome-shell | 50+ stars 门槛 | ⏸️ 等 stars |
| 🟢 P2 | Homebrew | 发现性提升 | ⏸️ 暂缓，等 200+ stars |

---

## 🎯 Karma 攒分手册

### HN（目标：3-5 天解锁 Show HN）

**每天 15 分钟，挑 1-2 个 thread 写实质评论：**

- ✅ 推荐 thread 类型：
  - Show HN 终端/CLI 工具类
  - "为什么我从 X 切换到 Y" 类编辑器讨论
  - Markdown 渲染/编辑器对比
  - terminal 工作流分享
- ❌ 避免：纯顶帖、调侃、政治、争议话题
- 🎯 评论质量 > 数量：一条 100 字的真知灼见 > 十条 "Great post!"

### Reddit（目标：2-3 周到 50 karma）

**sub 选择（按门槛从低到高）：**
- r/Markdown — 通常门槛最低，技术讨论氛围好
- r/terminal — 小众但精准
- r/commandline — 主流但严格（≥50 karma）

**每天动作：**
- 在 1-2 个 sub 留 1-2 条实质评论
- 风格：技术讨论、不打广告、真实用户身份
- ⚠️ **绝对不要**在新号推广期内提到自己的项目 — 一旦被识别会进 shadowban

### 解锁后立即动作

- HN：发 Show HN（用现有英文通稿，带 v1.0.9 / v1.0.10 release news）
- Reddit：r/commandline + r/Markdown + r/terminal 三连发，同步英文通稿去掉 "Show HN:"

---

## 附录 A：竞品数据

### GitHub

| 项目 | Stars | 定位 |
|------|-------|------|
| Yazi | 39.2k | 终端文件管理器 |
| Glow | 25.7k | Markdown 阅读器 |
| Micro | 25.3k | 终端编辑器 |
| frogmouth | 3.2k | Markdown 浏览器 |
| markln | 56 | Markdown 编辑器（Python，分屏） |

### Hacker News

| 项目 | Points | 成功要素 |
|------|--------|----------|
| Micro | 349 | 简洁标题 + 成熟产品 |
| Micro (第二次) | 324 | 社区已有认知 |
| Glow | 283 | 一句话说清 + 差异点 |
| Doxx | 261 | 借势 Glow + 痛点驱动 |
| Fresh | 187 | 挑战现状叙事 |
| C-edit | 168 | 复古情怀 |
| Ki Editor | 138 | 一个明确卖点 |

### microNeo 独特定位

| | microNeo | Micro | Glow | frogmouth | markln |
|--|:---------:|:-----:|:----:|:---------:|:------:|
| Markdown 渲染 | ✅ | ❌ | ✅ | ✅ | ✅ |
| 可编辑 | ✅ | ✅ | ❌ | ❌ | ✅ |
| 同一窗口 | ✅ | — | — | — | ❌ 分屏 |
| 语言 | Go | Go | Go | Python | Python |
| 单二进制 | ✅ | ✅ | ✅ | ❌ | ❌ |

> **microNeo = Micro 的编辑能力 + Glow 的渲染能力，合二为一。**
> 同窗口渲染 + 可编辑，全网无竞品。

---

## 🆕 新目标待提交（按优先级，6/19 全网盘查）

> 整理逻辑：先 **P0 三个**（高 stars + 高 fit + 活跃维护），效果立等可见。P1 等 v1.0.x 涨粉后再开。P2 卡在硬性门槛。

### P0 — 立即提 PR（这周内）

| 仓库 | Stars | 维护活跃度 | 收录位置 | 备注 |
|------|-------|------------|----------|------|
| **mundimark/awesome-markdown-editors** | 2.1k | 🟢 **6/16 push** | Desktop Editors → Linux（交叉 Universal） | **最精准的 fit**。2026 新规：先加 [UPCOMING.md](https://github.com/mundimark/awesome-markdown-editors/blob/master/UPCOMING.md)，主列表定期同步 |
| **mundimark/awesome-markdown** | 1.9k | 🟢 6/11 push | Editors 段 | 跟上面同 owner，PR #124 失效时退路 |
| **hackstoic/golang-open-source-projects** | 11.5k | 🟢 5/31 push | Terminal tools / Editor | **中文 Go 开源榜单**，中文 README，对中文开发者更精准 |

### P1 — 等 v1.0.x 做出小 milestone 再提（2-3 周后）

| 仓库 | Stars | 收录位置 | 备注 |
|------|-------|----------|------|
| **herrbischoff/awesome-macos-command-line** | 30.7k | Editors 段 | 30k+ stars 大列表，门槛高，需要更多 stars + Mac demo gif |
| **agarrharr/awesome-cli-apps** | 19.8k | Text Editors + Markdown | **重提**：#1142 被无理由 close，**等 200+ stars 后用极简 PR 重提**（详见下方"重提策略"） |
| **thegdsks/awesome-modern-cli** | 368 | Text Editors | PR #20 还开着，月底前没反应就 close + 重新开 |
| **BubuAnabelas/awesome-markdown** | 933 | Editors 段 | PR #124 还开着，owner 2024-08 后无 push — 可能僵尸，备选 mundimark |

### P2 — 暂缓（卡在 star 数 / 维护状态）

| 仓库 | Stars | 阻塞原因 | 解锁条件 |
|------|-------|----------|----------|
| **alebcay/awesome-shell** | 37.1k | **明确要求 50+ stars**（[CONTRIBUTING](https://github.com/alebcay/awesome-shell/blob/master/CONTRIBUTING.md)） | 50 stars（10 → 50） |
| avelino/awesome-go | 175.8k | **只收 libraries，不收 apps** | 永久放弃 |
| inputsh/awesome-linux | 5.1k | 2023-02 后无 push，已 abandoned | 跳过 |
| herrbischoff/awesome-command-line-apps | 4.2k | 跟 agarrharr 那份重复 | 跳过 |
| cdleon/awesome-terminals | 2.8k | 专注 terminal emulators，不适合 | 跳过 |
| **learnbyexample/TUI-apps** | 991 | **不是 curated list**，是个人 CLI 学习项目 | 跳过 |

### 重提策略：awesome-cli-apps #1142

**事件回顾**：6/8 12:35 提交 → 6/8 18:45 被 jneidel close（**6 小时内**，0 评论，0 review）
**同期数据**：同维护者 6 月合并了 15 个 PR（#1087, #1090, #1091, #1096, #1097, #1116, #1134, #1135, #1136, #1139, #1141, #1149, #1154 等），合并率约 50%

**为什么被 close**（猜测）：
- microNeo 当时只有 4 organic stars
- 描述太长 / 跟已有 Micro fork 重复
- 维护者最近在批量清理"低 stars + 长描述"PR

**重新打开策略**：
1. **等 200+ stars 后**再提（v1.0.x 持续涨粉）
2. 改用**极简 PR 描述**（3-5 行）：

```
Adds microNeo to Text Editors and Markdown sections.

microNeo is a terminal Markdown editor that renders and edits 
in the same window (no split panes). Open any .md file, see it 
rendered, click to edit. 1k+ stars on the parent Micro project.
```

3. **只加 1 个位置**（Text Editors），不再同时加 Markdown 段（之前双段可能触发了 spam 检测）
4. PR 标题缩短到 60 字符以内

### 备份方案

如果 2 周后仍无响应：
- **awesome-modern-cli**：owner 月更节奏，等月底不响应就 close 重开
- **awesome-markdown**：BubuAnabelas 2024-08 后无 push，**主推 P0 的 mundimark 两个 repo 作为替代**

---

## 📝 PR 模板（即用即改）

### 模板 1：通用 awesome list 提交（英文）

**PR Title**（60 字符内）：
```
Add microNeo to <Section Name> section
```

**PR Body**：
```markdown
Adds [microNeo](https://github.com/sollawen/microNeo) to the <Section Name> section.

microNeo is a terminal Markdown editor that renders and edits in the same 
window. Open any `.md` file → see it rendered → click anywhere to edit the 
source. No split panes.

- Built in Go, single binary
- Works as `$EDITOR` for Claude Code, Yazi, etc.
- One-line install: `curl -fsSL .../install.sh | sh`

Differentiation from existing entries:
- Micro/nano: no Markdown rendering
- Glow/frogmouth: read-only viewers
- vim + plugins: split panes (terminal screens aren't wide)
```

**配套操作**：
- PR 同时给 README 加 `[![awesome-xxx](badge)](URL)` badge
- 检查 GitHub topics 是否需要调整（**当前 12 个已设置，详见 附录 C**）

### 模板 2：中文 awesome 列表提交

**PR Title**：
```
添加 microNeo - 终端 Markdown 编辑器
```

**PR Body**：
```markdown
#### sollawen - [GitHub](https://github.com/sollawen)
* :white_check_mark: [microNeo](https://github.com/sollawen/microNeo)：终端 Markdown 编辑器，**同窗口渲染+编辑**。打开 .md 文件即看到排版效果，点击即可编辑，无需分屏。Go 单二进制，一行命令安装
```

### 模板 3：跟进被 close 的 PR（重新打开）

**适用**：awesome-cli-apps 之类的
- **不要在原 PR 下争论**
- **直接新开 PR**（用上面模板 1 的极简版）
- 描述中**不引用**原 PR 编号

---

## 附录 B：awesome PR 完整记录

### ✅ 已 merge（2）

| 仓库 | Stars | PR | Merge 时间 | 流量贡献（14 天） |
|------|-------|-----|-----------|------------------|
| rothgar/awesome-tuis | 19.3k | [#713](https://github.com/rothgar/awesome-tuis/pull/713) | 2026-06-16 | **主长尾**（预期 10-50 浏览/天） |
| 1c7/chinese-independent-developer | 48.8k | [#975](https://github.com/1c7/chinese-independent-developer/pull/975) | 2026-06-09 | 6-08 触发 79 clones uniques 峰值 |

### 🟡 待跟进（3）

| 仓库 | Stars | PR/Issue | 提交时间 | 状态 | 跟进动作 |
|------|-------|----------|----------|------|----------|
| thegdsks/awesome-modern-cli | 368 | [#20](https://github.com/thegdsks/awesome-modern-cli/pull/20) | 6/8 | OPEN 12 天 | 等月底，不响应则重开 |
| BubuAnabelas/awesome-markdown | 933 | [#124](https://github.com/BubuAnabelas/awesome-markdown/pull/124) | 6/8 | OPEN 12 天 | owner 2024-08 后无 push，可能僵尸，备选 mundimark |
| 521xueweihan/HelloGitHub | 160k | [Issue #3335](https://github.com/521xueweihan/HelloGitHub/issues/3335) | 6/8 | 等月刊审核 | 月刊发布日 7 月初 |

### ❌ 已关闭（1）

| 仓库 | Stars | PR | 关闭时间 | 关闭原因 | 后续 |
|------|-------|-----|----------|----------|------|
| agarrharr/awesome-cli-apps | 19.8k | [#1142](https://github.com/agarrharr/awesome-cli-apps/pull/1142) | 2026-06-08（6h 内） | 无理由 close | **200+ stars 后重提**（用极简模板） |

### 🆕 待提交（3 — P0）

| 仓库 | Stars | 拟加位置 | 优先级 | 提交模板 |
|------|-------|----------|--------|----------|
| mundimark/awesome-markdown-editors | 2.1k | Desktop Editors → Linux + Universal | 🔴 P0 | 模板 1 |
| mundimark/awesome-markdown | 1.9k | Editors 段 | 🔴 P0 | 模板 1 |
| hackstoic/golang-open-source-projects | 11.5k | Terminal tools / Editor | 🔴 P0 | 模板 2（中文） |

### ⏸️ 暂缓（等 stars / 维护者活跃）

| 仓库 | Stars | 解锁条件 | 当前状态 |
|------|-------|----------|----------|
| alebcay/awesome-shell | 37.1k | 50+ stars（**需 5x 增长**） | 10 stars，等 v1.0.x 涨粉 |
| herrbischoff/awesome-macos-command-line | 30.7k | 200+ stars + Mac demo gif | 当前没 Mac-specific 卖点 |
| avelino/awesome-go | 175.8k | 不可解锁（只收 libraries） | 永久放弃 |
| inputsh/awesome-linux | 5.1k | 维护者复活 | 2023-02 abandoned，跳过 |
| learnbyexample/TUI-apps | 991 | 不是 curated list | 跳过 |

---

## 附录 C：SEO 关键词覆盖矩阵（2026-06-19 建立）

> 这次 SEO 改动的核心思想：**让用户搜什么都能找到 microNeo**。
> 下面这个矩阵是所有抢的搜索词及抢的方式。

### 主关键词分类

| 类别 | 关键词 | 抢的方式 |
|---|---|---|
| **品牌词** | `microNeo` | GitHub repo、awesome-tuis badge、README |
| **父项目词** | `Micro` / `micro editor` | Q5 (vs Micro) + 继承描述 |
| **竞品词** | `Glow` / `vim` / `nano` | Q4 (vs Glow) + 横向对比表 |
| **USP 词** | `same window` / `no split` / `live preview` / `renders and edits` | Q2、Headline、Description |
| **技术词** | `Go` / `single binary` / `syntax highlighting` | Description、Q8、topic `go` |
| **用例词** | `Claude Code` / `opencode` / `$EDITOR` / `Yazi` | Q7 (Claude/opencode) + Q8 |
| **痛点词** | `terminal markdown editor` / `code editor terminal` | Tagline、Q1、Q8 |
| **平台词** | `terminal` / `TUI` / `Linux` / `cross-platform` | topics: `terminal`, `tui`, `linux`, `cross-platform` |
| **AI 编程词** | `Claude Code` / `claude code editor` | Q7、topic `claude-code` |

### 关键词 vs 落地位置对照表

| 关键词 | README | FAQ | Description | Topics | Headline |
|---|:---:|:---:|:---:|:---:|:---:|
| terminal markdown editor | ✅ | Q1, Q2, Q3 | ✅ | ✅ | ✅ |
| renders and edits in same window | — | Q2, Q5, Q8 | ✅ | — | ✅ |
| live preview | — | Q2, Q3 | — | — | ✅ |
| no split panes / no split | — | Q2 | — | — | ✅ |
| single Go binary | ✅ | Q3 | ✅ | ✅ (`go`) | — |
| syntax highlighting | — | Q8 | ✅ | ✅ | ✅ |
| 100+ languages | — | Q8 | ✅ | — | ✅ |
| $EDITOR / Claude Code / opencode | — | Q7 | ✅ | ✅ (`claude-code`) | ✅ |
| cross-platform | — | Q5 (via Micro) | — | ✅ | — |
| vs Glow | — | Q4 | — | — | — |
| vs Micro | — | Q5 | — | — | — |

### Topics 决策记录

| 决策 | 理由 |
|---|---|
| **去掉 `markdown`** | 跟 markitdown (155k)、firecrawl (135k) 同台，被淹没 |
| **加 `markdown-editor`** | 跟 foam (17k)、Milkdown (11k) 同台，精准 |
| **加 `syntax-highlighting`** | 抢 "syntax highlighting terminal" 长尾 |
| **加 `markdown-rendering`** | 跟 USP"同窗口渲染+编辑"对齐 |
| **加 `claude-code`** | 抢 Claude Code 用户增长红利 |
| **加 `cross-platform`** | 跟 `linux` 互补（macOS/Windows 用户） |
| **保留 `go`** | 行业惯例 + 组合搜索过滤 + 技术信号 |
| **保留 `editor`/`terminal`/`tui`/`cli`/`linux`/`micro`** | 基本身份标签，不删 |

### 预期流量来源结构（6 月底 vs 8 月底）

| 流量来源 | 6/19（SEO 前） | 预期 8 月底（SEO 生效后） |
|---|---|---|
| awesome-tuis 长尾 | 10-50 浏览/天 | 10-50 浏览/天（稳定） |
| **Google organic 搜索** | **几乎为 0** | **20-100 浏览/天**（新 ✅） |
| Chinese awesome list 长尾 | 5-20 浏览/天 | 5-20 浏览/天（稳定） |
| HN / Reddit（解锁后） | 0 | 50-500 stars 一次性爆发 |
| 直接访问 / 外链 | 同事 + 偶然 | organic + 同事 |
| **合计** | 15-70 浏览/天 | 50-200 浏览/天 |

### 监控指标（接下来 4 周看）

| 指标 | 目标 | 测量方式 |
|---|---|---|
| Google 索引量 | 1-2 周内 README 进入索引 | `site:github.com/sollawen/microNeo` |
| PAA 命中 | 4 周内 FAQ Q&A 出现在 PAA | 手动搜 "best terminal markdown editor" 等关键词 |
| Organic visits | 4 周内 GitHub traffic referrers 出现 google.com | `gh api /traffic/popular/referrers` |
| Stars 增长 | 4 周内 organic stars +5-10 | 排除同事后看新增 |

### 复盘节奏

- **每周日** 跑一次 `gh api /traffic/popular/referrers` 看 organic 占比
- **每 2 周** 手动搜 5 个目标关键词看 PAA 进度
- **每 4 周** 更新本附录，记录实际 vs 预期偏差

**所有数据记录在** [`seo-progress.md`](./seo-progress.md) **（单独的 progress log，T0 baseline 已建立）**

