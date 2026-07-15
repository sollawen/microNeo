---
name: changelog
description: Writes/updates CHANGELOG.md entries. Use when user asks to add a new version entry, update the changelog, or summarize recent changes.
---

## Core principle

**只讲用户可见的功能变化，不写技术实现细节。**

用户读 changelog 是想知道「这个版本我能用到什么新东西、什么变了、什么修好了」。实现层面用了什么 sentinel、归一化放哪一层、调了哪个内部函数 —— 这些属于 commit message 和设计文档，不属于 changelog。

判断标准：
- 如果一个变化用户在界面上**看不到、操作不到、感知不到**，它通常不该单独占一条 changelog bullet。
- **较大规模的内部重构**值得记一条（放 **Changed**），让读者了解架构演进和潜在的升级影响；琐碎重构不记。

## Format (hard constraint)

`## [X.Y.Z] - YYYY-MM-DD` —— 方括号、无 `v` 前缀、ISO 日期。

这是硬约束：`tools/extract-changelog.sh` 用 `substr($0,1,5+len(v)) == "## [" v "]"` 精确匹配来提取 release notes。格式错了 release workflow 会失败。

- 新版本条目**插在最顶部**（最新在上）。
- 日期用该版本最后一个 commit 的日期。
- 分类用**加粗** heading，顺序：**Added** → **Changed** → **Fixed** → **Removed** → **Docs**。用不到的分类直接省略，不要留空 heading。

## Bullet writing

- 一条 bullet = 一个变化。动名词或名词开头，先说「是什么」再说「有什么用」。
- 句尾可点一句用户价值（为什么 / 解决了什么），但不要展开怎么做。
- 命令、配置项、文件名用反引号。
- 较大规模的内部重构（即使无用户可见行为变化）应记入 **Changed**，一句话点明重构的范围和动机（如「重构 X 模块以提升 Y」），不写实现细节；琐碎重构不记。

### 反例（实现细节泄漏，不要这样写）

> - `SelectPane.Open` gains `maxVisible` + `wrap` params. Overflowing lists scroll with `▲▼` indicators drawn in the right padding column. `FloatFrame` is unchanged — scrolling is the concrete popup's business, not the framework's.
> - `FloatFrame.Open` normalization lives in `Open` (input layer), not in the pure-geometry `expandAnchor`; existing callers passing `Y >= 0` are unaffected.

### 正例（只讲用户能感知的）

> - SelectPane supports list scrolling (with `▲▼` overflow indicators) and caller-configured viewport height / wrap behavior.
> - FloatFrame supports bottom-anchored popups via negative `anchor.Y`, so popups can snap to just above the status line.

## Workflow

1. `git log <last-tag>..HEAD --format='%h | %ad | %s' --date=short` 列出待发布提交。
2. 按 Added / Changed / Fixed / Removed / Docs 归类（docs: / chore: 前缀的多数进 **Docs** 或省略）。
3. 必要时 `git show <hash>` 看提交正文确认变化的**用户面**（不是实现面）。
4. 在 CHANGELOG.md 顶部插入新 `## [X.Y.Z]` 段。
5. 拿不准一条要不要写时，默认不写 —— changelog 宁精勿滥。

## Language

English only.
