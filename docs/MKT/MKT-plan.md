# microNeo Marketing Plan

> Last updated: 2026-06-19

---

## 📣 状态更新（2026-06-19）

**最近里程碑（6/16）：**
- ✅ **awesome-tuis PR #713 已 merge**（[链接](https://github.com/rothgar/awesome-tuis/pull/713)）—— **第一个英文 awesome 列表收录**（19.3k ⭐，由 Justin Garrison 合并）
  - README badge 已加：`https://github.com/rothgar/awesome-tuis`
  - 当前流量贡献：14 天 96 次站内 referrer，awesome-tuis 应是主长尾来源
  - 预期长期效果：每天 10-50 浏览，持续带来 organic stars

**待跟进（3 个 PR pending）：**
- 🟡 awesome-cli-apps #1142（19.7k ⭐）— 6/8 提交，10 天无动静 → 礼貌 ping 维护者
- 🟡 awesome-modern-cli #20（361 ⭐）— 6/8 提交，月更节奏
- 🟡 awesome-markdown #124（931 ⭐）— 6/8 提交，月更节奏

**见下方 "立即行动（7 天冲刺）" 获取完整计划。**

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
- binary 下载数 ≈ 真实装机数，是 install.sh 的代理指标（**当前 ~10-15/14 天**）
- 改成 `git clone` 会把装机数砍 5-10×，得不偿失
- 所有成功的 CLI 工具（bat/fd/ripgrep/zoxide）都走 `curl | sh`

---

## 📊 数据快照（2026-06-19 GitHub API 抓取）

| 指标 | 值 | 说明 |
|---|---|---|
| 总 stars | 10 | 6 来自同事，**organic = 4** |
| 14 天 unique visitor | 34 | 真的点进 GitHub 看 README/demo 的人 |
| 14 天 git clone unique | 430 | 大部分是开发者/CI/机器人，非真用户 |
| 14 天 binary 下载（install.sh 代理） | **~10-15** | 真的装了 binary 准备用的人 |
| **visitor → star 转化率** | **12%** | 健康（行业平均 1-3%） |
| 全部 release binary 总下载 | 45 | 终身估算 30-40 unique user |
| Top referrer | github.com | 无 HN / X / 搜索引擎入口 |

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
| 🔴 P0 | **dev.to 长文** | SEO 长尾 50-200 stars | 📋 Day 1（今天） |
| 🔴 P0 | **Twitter/X thread** | 病毒传播 0-500 stars | 📋 Day 1-2 |
| 🔴 P0 | **V2EX 分享创造** | 中文圈 50-200 stars | 📋 Day 2-3 |
| 🔴 P0 | awesome-tuis | 长期每天 10-50 浏览 | ✅ 已 merge |
| 🟡 P1 | **掘金长文** | 中文长尾 100-500 浏览 | 📋 Day 3-5 |
| 🟡 P1 | HN 攒 karma | 解锁 Show HN | 🔄 后台 3-5 天 |
| 🟡 P1 | Reddit 攒 karma | 解锁 r/commandline | 🔄 后台 2-3 周 |
| 🟡 P1 | awesome-cli-apps PR #1142 | 长期少量浏览 | ✅ 已提交 10 天，跟进中 |
| 🟢 P2 | HN Show HN（解锁后） | 0-300 stars | 📋 解锁后 |
| 🟢 P2 | Reddit r/commandline（解锁后） | 50-200 浏览 | 📋 解锁后 |
| 🟢 P2 | awesome-modern-cli | 长期少量浏览 | ✅ PR 已提交 |
| 🟢 P2 | awesome-markdown | 同上 | ✅ PR 已提交 |
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

## 附录 B：awesome PR 记录

| 仓库 | Stars | 维护者更新频率 | PR | 加的位置 | Merge 概率 |
|------|-------|----------------|-----|---------|-----------|
| rothgar/awesome-tuis | 19.3k | 活跃（最近 merge 6/3） | [#713](https://github.com/rothgar/awesome-tuis/pull/713) | Editors | ✅ **已 merge (6/16)** |
| **agarrharr/awesome-cli-apps** | 19.7k | 很活跃（6/4~6/7 天天 merge） | [#1142](https://github.com/agarrharr/awesome-cli-apps/pull/1142) | Text Editors + Markdown | 中高 |
| thegdsks/awesome-modern-cli | 361 | 月更（最近 merge 5/6） | [#20](https://github.com/thegdsks/awesome-modern-cli/pull/20) | Text Editors | 高 |
| **1c7/chinese-independent-developer** | **48.8k** | **很活跃（6/5~6/8 天天 merge）** | [**#975**](https://github.com/1c7/chinese-independent-developer/pull/975) | 程序员版面 | ✅ 已 merge（6/9） |
| **521xueweihan/HelloGitHub** | **160k** | 月刊发布，审核周期 1-4 周 | [**Issue #3335**](https://github.com/521xueweihan/HelloGitHub/issues/3335) | Go 类目投稿 | 中高（需审核） |
