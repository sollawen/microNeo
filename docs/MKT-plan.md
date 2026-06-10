# microNeo Marketing Plan

> Last updated: 2026-06-10

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

## 执行进度

### ✅ Day 1 — 2026-06-08（周日）

| 任务 | 状态 | 链接/说明 |
|------|------|-----------|
| 竞品调研（HN 数据、GitHub stars） | ✅ 完成 | 见附录 A |
| 确定核心卖点：同窗口渲染 + 编辑 | ✅ 完成 | 全网无竞品，验证过 |
| awesome-tuis PR | ✅ CI 通过，等待 merge | [PR #713](https://github.com/rothgar/awesome-tuis/pull/713)（19.3k ⭐） |
| awesome-cli-apps PR | ❌ 被关闭（无评论无理由） | [PR #1142](https://github.com/agarrharr/awesome-cli-apps/pull/1142)（19.7k ⭐） |
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

### 🔶 Day 2 — 2026-06-09（周一）晚上发布

| 任务 | 时间 | 状态 | 说明 |
|------|------|------|------|
| **HN Show HN 发帖** | 21:16 | ❌ 被拒 | 新号限制，需先攒 karma，一周后重试 |
| **Reddit r/commandline** | 21:30 | ⏳ 等审核 | 已发帖，flair: Terminal User Interface，注意 Rule 5 限制新项目 |
| **V2EX** | — | ❌ 需邀请码 | 注册需要邀请码，待获取 |

### 🔶 本周核心任务（2026-06-10 ~ 2026-06-16）

**目标：去目标社区混脸熟，攒 karma 和存在感**

**方法：**
1. 在 Chrome 里浏览合适的帖子
2. 找到能聊的帖子，发表评论/回复
3. 不在乎有没有人看，只在乎积累真实经验
4. 使用 reddit-tools skill 辅助

**重点版块：**
| 社区 | 版块 | 说明 |
|------|------|------|
| Reddit | r/opencode | opencode 用户主场，6 个月使用经验完全在行 |
| Reddit | r/ClaudeAI | 200k+ 成员，Claude Code 使用经验，流量大 |
| Reddit | r/commandline | 通用命令行工具受众，适合分享 microNeo 使用体验 |
| Reddit | r/terminal | 通用终端社区 |
| Reddit | r/vim | 终端编辑器用户，可能聊 markdown preview |
| HN | newest 页面 | 程序员密度最高，贴子< 24h 评论少，适合混脸熟 |

**工具：reddit-tools skill（`~/.pi/agent/skills/reddit-tools/`）+ hn-tools skill（`~/.pi/agent/skills/hn-tools/`）**

---

### ✅ Day 3 — 2026-06-10（周二）Reddit + HN 调研 + 实际发帖

| 任务 | 状态 | 说明 |
|------|------|------|
| Reddit 调研方法 | ✅ 完成 | 建立了 reddit-tools skill，含 4 个脚本 |
| r/tui 帖子列表浏览 | ✅ 完成 | 52 个帖子，已读 3 个 |
| r/ClaudeAI 调研 | ✅ 完成 | 发现 r/ClaudeAI (200k+ 成员) 比 r/tui (8.7k) 适合新手攒 karma |
| **r/ClaudeAI 发帖 × 2** | ✅ 完成 | ① Claude productivity hack ② ISP battle 帖子（用户手动发） |
| r/opencode 发帖 | ✅ 完成 | 2 条评论（token 消耗 + plan/build 模型选择） |
| HN 调研 | ✅ 完成 | 发现 HN 搜索用 Algolia，news.ycombinator.com/search 常被 block |
| **HN 发帖 × 1** | ✅ 完成 | AI 替代员工是胡扯 |
| **创建 hn-tools skill** | ✅ 完成 | 4 个脚本（search + extract-post-list + extract-post + post-comment） |
| 自动发帖脚本 | 🔶 Reddit 不稳定 | post-comment.js 展开步骤不一致，用户经常需手动贴 |
| HN search 脚本 | ✅ 完成 | 用 Algolia API 绕过 block |

#### Day 3 详细活动

| 时间 | 帖子 | 评论内容 | 状态 |
|------|------|----------|------|
| 下午 | HN Leash 帖子 | "I never use Safari on my iPhone either, the screen is too small. iPad is much better for browsing." | ✅ |
| 下午 | HN AI replaces employees | "AI replacing human workers is mostly BS. AI writes a ton of code way faster than I can type. But the code is garbage." | ✅ |
| 下午 | HN Terminal writing environment | "This is impressive! Using Git to track notes is a clever idea." | ✅ |
| 下午 | HN Pi 文章 | "I think Pi is pretty cool. Everything is so simple, but the simpler it is, the more efficient." | ✅ |
| 下午 | HN How are you preserving your skills | "Before AI writes code, only discuss architecture, main flow, data structures. Then let AI code." | ✅ |
| 下午 | HN OpenCode + microcontroller | "In my company, most software engineers are still copy-pasting from ChatGPT." | ⚠️ Rate limited |

#### hn-tools skill 更新

| 脚本 | 更新内容 |
|------|----------|
| search.js | 支持 `date`/`popularity` 排序，修复 API endpoint（`search_by_date`） |
| extract-post.js | 修复 site 正则、添加 `| next` 评论匹配、优化 post 正文提取 |
| post-comment.js | textarea.value + dispatchEvent + form.submit() 三步法 |
| SKILL.md | 区分静默操作 vs 导航操作 |

#### hn0610.md 帖子文档

创建了 `docs/hn0610.md`，记录 5 个适合评论的 HN 帖子，含全文翻译。

#### HN Rate Limit 发现

- **触发条件**：几分钟内发 6+ 条评论
- **URL 特征**：`comment-toofast`
- **恢复时间**：等待 10-15 分钟

#### Reddit Karma 机制重要发现

- **Karma = 靠 upvote 攒的**，不是发帖数量。发 100 条没人点赞的回复 = 0 karma。
- **r/tui 只有 8.7k 成员**，技术小众圈，不懂技术硬发很尴尬，攒 karma 效率极低。
- **r/ClaudeAI 有 200k+ 成员**，流量大，Fable 5 发布期间讨论热，不需要专业知识，只要懂 Claude Code 使用就能聊。
- **r/opencode 是 opencode 用户的主场**，用户有 6 个月真实使用经验，完全在行。

#### 结论：攒 karma 优先去大流量、纯闲聊、人人有话说的社区，不是在技术上死磕。

### 📋 Day 4+ 待办

| 任务 | 说明 | 优先级 |
|------|------|--------|
| HN 攒 karma | 先逛 HN 发评论，攒够再发 Show HN | P1 |
| HN rate limit 恢复 | 等 10-15 分钟后继续发评论 | P1 |
| Reddit 审核 | 关注 r/commandline 帖子是否通过 | P1 |
| V2EX 发帖 | 需先搞到邀请码，注册后发分享创造节点 | P1 |
| **HN 发帖（帖子2）** | OpenCode + microcontroller development，待 rate limit 恢复后重发 | P1 |
| **Reddit 工具链完善** | 改进 post-comment.js，让自动发帖更稳定 | P2 |
| 掘金文章 | 先注册账号，Go + 终端工具受众 | P2 |
| dev.to 文章 | 先注册账号，英文版个人故事 | P2 |
| Homebrew formula 提交 | 等 200+ stars 再尝试 | P3 |
| Discord 推广 | 加入 CLI/TUI/Go 相关服务器，适当推广 | P2 |

---

## 推广通稿（所有渠道共用）

> 发布前按平台规则微调，Last updated: 2026-06-09

### 正文

```
I use Micro to edit Markdown files, and Glow to read them. The constant switching drove me crazy.

Every other solution either can't edit (Glow, frogmouth) or splits your screen in two (vim plugins, GUI editors). Terminal screens aren't wide to begin with — splitting them is painful.

So I forked Micro and added a Markdown rendering layer. Open any .md file, you see it rendered. Click anywhere to edit the source. No split panes.

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

### 待起草

- **V2EX 中文版** — 发在分享创造节点，用中文写
- **掘金文章** — "I forked Micro to make..." 个人故事角度，Go + 终端工具受众
- **dev.to 文章** — 同掘金，英文版

---

## README 优化要点（Day 2 执行）

1. **去掉 "AI era"** — HN 用户反感 AI 炒作，用实际痛点切入
2. **压缩 Why microNeo** — 5 个小节 → 2-3 句话 + 对比表格
3. **首屏一句话说明** — 先说产品是什么，再讲为什么
4. **截图放最前面** — 视觉冲击力 > 文字
5. **不需要 GIF** — microNeo 是通用编辑器，核心卖点"同窗口渲染+编辑"静态截图就能传达，没有特别需要演示的操作

---

## 推广渠道优先级

| 优先级 | 渠道 | 预期效果 | 状态 |
|--------|------|----------|------|
| 🔴 P0 | awesome-tuis | 长期每天 10-50 浏览 | ✅ PR 已提交 |
| 🔴 P0 | awesome-cli-apps | 同上 | ✅ PR 已提交 |
| 🔴 P0 | HN Show HN | 0-300+ stars | 📋 今晚 20:00 |
| 🟡 P1 | Reddit r/ClaudeAI | 200k+ 成员，流量大，适合攒 karma | ✅ 已发 2 条评论 |
| 🟡 P1 | Reddit r/opencode | opencode 用户主场，适合发产品体验帖 | 📋 准备发「Opencode is great」回帖 |
| 🟡 P1 | Reddit r/commandline | 50-200 浏览 | ✅ 已发，等审核（注意 Rule 5 限制新项目） |
| 🟡 P1 | V2EX | 50-200 浏览 | 📋 Day 3 |
| 🟡 P2 | Reddit r/tui | 8.7k 成员，技术小众，不适合新号攒 karma | ⚠️ 暂缓，先去大流量社区 |
| 🟢 P2 | dev.to 文章 | ❤️ 10-50 | 📋 Week 2 |
| 🟢 P2 | 掘金文章 | 100-500 浏览 | 📋 Week 2 |
| 🟢 P2 | awesome-modern-cli | 长期少量浏览 | ✅ PR 已提交 |
| 🟢 P2 | Homebrew | 发现性提升 | ⏸️ 暂缓，等 200+ stars 再尝试官方 homebrew-core |

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
| rothgar/awesome-tuis | 19.3k | 活跃（最近 merge 6/3） | [#713](https://github.com/rothgar/awesome-tuis/pull/713) | Editors | 高 |
| ~~agarrharr/awesome-cli-apps~~ | 19.7k | ~~很活跃（6/4~6/7 天天 merge）~~ | [#1142](https://github.com/agarrharr/awesome-cli-apps/pull/1142) | Text Editors + Markdown | ❌ 被关闭，无理由 |
| thegdsks/awesome-modern-cli | 361 | 月更（最近 merge 5/6） | [#20](https://github.com/thegdsks/awesome-modern-cli/pull/20) | Text Editors | 高 |
| **1c7/chinese-independent-developer** | **48.8k** | **很活跃（6/5~6/8 天天 merge）** | [**#975**](https://github.com/1c7/chinese-independent-developer/pull/975) | 程序员版面 | ✅ 已 merge（6/9） |
| **521xueweihan/HelloGitHub** | **160k** | 月刊发布，审核周期 1-4 周 | [**Issue #3335**](https://github.com/521xueweihan/HelloGitHub/issues/3335) | Go 类目投稿 | ⏳ 审核中 |
| **rothgar/awesome-tuis** | **19.3k** | 活跃（最近 merge 6/3） | [PR #713](https://github.com/rothgar/awesome-tuis/pull/713) | Editors | ⏳ 等 merge |
| **thegdsks/awesome-modern-cli** | **361** | 月更（最近 merge 5/6） | [PR #20](https://github.com/thegdsks/awesome-modern-cli/pull/20) | Text Editors | ⏳ 等 merge |
