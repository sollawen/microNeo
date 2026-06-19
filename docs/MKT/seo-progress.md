# microNeo SEO Progress Log

> **Purpose**: Track organic search growth after the 6/19/2026 SEO overhaul.
> **Baseline (T0)**: 2026-06-19, captured immediately after all 7 SEO changes.
> **Refresh cadence**: Every Sunday (weekly)
> **Owner**: sollawen

---

## 📊 Running Dashboard

> Last update: **2026-06-19 (T0)**

### A. 真正装机数（最重要）

> ⚠️ **microNeo 是 `curl | sh` 安装，不是 `git clone`**。
> GitHub `clones` API 数的是开发者 / bots / 贡献者，**不代表真实用户**。
> 真实装机数 ≈ **GitHub Releases binary 下载数**（最接近的 proxy）。

| 指标 | 当前 | vs 4 周前 | 趋势 | 目标 (8 月底) |
|---|---:|---:|:-:|---:|
| **⭐ Binary 下载 (终身)** | **41** | — | 🟢 baseline | 200-500 |
| **⭐ Binary 下载 (7d)** | **20** | — | 🟢 baseline | 50-150 |
| **⭐ Binary 下载 (3d, awesome-tuis 后)** | **12** | — | 🟢 baseline | 30-100 |
| Binary 下载 / 天 (7d 均值) | ~2.9/天 | — | 🟢 baseline | 7-20/天 |

**按平台分布（终身 41 次）**：

| 平台 | 下载 | % |
|---|---:|---:|
| **macOS ARM64 (M1+)** | **20** | **49%** |
| Linux x64 | 15 | 37% |
| Windows x64 | 3 | 7% |
| Linux ARM64 | 1 | 2% |
| macOS Intel | 1 | 2% |
| Windows ARM64 | 1 | 2% |

> 🔑 **关键洞察**：M1+ Mac 用户是主要用户群（~一半）。**所有 demo、截图、教程都该默认 Mac 场景**。

### B. GitHub 仓库流量（次要）

| 指标 | 当前 | vs 4 周前 | 趋势 | 目标 (8 月底) |
|---|---:|---:|:-:|---:|
| Views (14d) | 271 | — | 🟢 baseline | 500-1000 |
| Unique visitors (14d) | 45 | — | 🟢 baseline | 100-200 |
| Clones (14d, **仅 dev/CI/bot** ⚠️) | 1,312 | — | 🟢 baseline | 1500-3000 |
| Unique cloners (14d, **仅 dev/CI/bot** ⚠️) | 441 | — | 🟢 baseline | 500-1000 |
| Stars (total) | 10 | — | 🟢 baseline | 30-50 |
| **Google referrer (14d)** | 5 | — | 🟢 baseline | 50-150 |
| Google referrer uniques (14d) | 2 | — | 🟢 baseline | 30-80 |
| Bing referrer (14d) | 2 | — | 🟢 baseline | 5-20 |
| PAA 命中关键词数 | 0 / 5 | — | 🟢 baseline | 2-4 / 5 |
| Google 索引页数 | ? | — | 🟢 baseline | README + 关键 FAQ |

### C. 转化漏斗

```
👀 看到 GitHub repo (Views)              271 / 14d    → 100%
   ↓
🔍 点进 repo (Visitors)                   45 / 14d    → 17%
   ↓
⭐ 觉得够好，给 star (Stars)              10 / lifetime → 3.7%
   ↓
📥 真的下载 binary (Real users)           41 / lifetime → 15%
   ↓
💻 真的用 ($EDITOR 实际配置)            估计 20-30   → ~50-70% of downloads
```

> 当前 visitor → install 转化率约 **15%**（41 安装 / 271 浏览）。
> 行业平均 visitor → install 约 2-5%。**说明来的人都是真目标用户** ✅。
> 接下来 SEO 要解决的是把 Views 从 271 拉到 500+。

---

## 🛠️ Measurement Playbook

> 每周日跑一次下面这套命令，结果贴到下面的 Log Entries。

### 1. GitHub Traffic + Releases（自动化）⭐ **关键**

```bash
echo "=== Views (14d) ==="
gh api repos/sollawen/microNeo/traffic/views \
  --jq '{total_count: .count, total_uniques: .uniques}'

echo ""
echo "=== Clones (14d, **不是真实装机** ⚠️) ==="
gh api repos/sollawen/microNeo/traffic/clones \
  --jq '{total_count: .count, total_uniques: .uniques}'

echo ""
echo "=== ⭐ Binary Downloads（真正装机 proxy）==="
gh api repos/sollawen/microNeo/releases --jq '
  [.[] | {
    tag: .tag_name,
    published: .published_at,
    total: ([.assets[] | select(.name | endswith(".sha") | not) | .download_count] | add // 0)
  }] | sort_by(.published) | .[] | "\(.tag)\t\(.published)\t\(.total)"
'

echo ""
echo "=== Lifetime total ==="
gh api repos/sollawen/microNeo/releases --jq '
  [.[] | .assets[] | select(.name | endswith(".sha") | not) | .download_count] | add // 0
'

echo ""
echo "=== 7d downloads ==="
gh api repos/sollawen/microNeo/releases --jq '
  [.[] | select(.published_at > (now | . - 604800 | todate)) | .assets[] | select(.name | endswith(".sha") | not) | .download_count] | add // 0
'

echo ""
echo "=== Top Referrers ==="
gh api repos/sollawen/microNeo/traffic/popular/referrers

echo ""
echo "=== Top Pages ==="
gh api repos/sollawen/microNeo/traffic/popular/paths

echo ""
echo "=== Stars ==="
gh repo view sollawen/microNeo --json stargazerCount
```

### 2. Google 索引检查（手动，浏览器）

```
site:github.com/sollawen/microNeo
```

记下：返回多少条结果、FAQ Q&A 是否在 snippet 里

### 3. PAA 检查（手动，浏览器）

依次搜索以下 5 个目标关键词，每个看 PAA 是否出现 microNeo 链接：

1. `best terminal markdown editor`
2. `terminal markdown editor with live preview`
3. `syntax highlighting terminal`
4. `claude code editor`
5. `micro editor alternative`

记录：5 个里有几个出现了 microNeo

### 4. Google Search Console（如果已绑定）

每周看：
- Impressions（搜索展示次数）
- Clicks（点击次数）
- Average CTR
- Average Position

---

## 📅 Schedule（接下来 8 周）

| 周 | 日期 | Day # | 重点看 | 备注 |
|---|---|---|---|---|
| W0 | 2026-06-19 (T0) | D+0 | **Baseline** | 7 项 SEO 改动刚完成 |
| W1 | 2026-06-26 | D+7 | Google 重新爬、topics 索引 | 第一次看 organic |
| W2 | 2026-07-03 | D+14 | FAQ PAA 是否出现 | 关键拐点 |
| W3 | 2026-07-10 | D+21 | organic 流量曲线 | 应开始有趋势 |
| W4 | 2026-07-17 | D+28 | **第一个月报**：vs baseline | 与 v2.0 launch 计划对齐 |
| W5 | 2026-07-24 | D+35 | 持续 organic 增长 | |
| W6 | 2026-07-31 | D+42 | v2.0 launch 前夕 | pre-launch 预热数据 |
| W7 | 2026-08-07 | D+49 | **v2.0 launch 当周** | Show HN + Reddit 解锁后 |
| W8 | 2026-08-14 | D+56 | **第二个月报**：organic + HN/Reddit 双引擎 | |

---

## 📝 Log Entries

> 每条 entry 一个 H3 section，模板：
> ```
> ### YYYY-MM-DD (Day+N) — Title
> **Measurement date**: ...
> **Raw data**:
> | 指标 | 数值 | vs 上一期 | vs baseline |
> **Analysis**: ...
> **Action items**: ...
> ```

---

### 2026-06-19 (Day 0, T0) — Baseline Captured 🟢

**Measurement date**: 2026-06-19 23:59 UTC
**Context**: 7 项 SEO 改动刚完成 30 分钟内

**Raw data**:

**⭐ 真正装机数（GitHub Releases binary downloads）**：

| Release | Published | Downloads |
|---|---|---:|
| v1.0.0 | 2026-06-08 | 4 |
| **v1.0.2** | 2026-06-09 | **12** ← 最高 |
| v1.0.3 | 2026-06-09 | 2 |
| v1.0.4 | 2026-06-11 | 3 |
| v1.0.5 | 2026-06-15 | 8 |
| v1.0.6 | 2026-06-17 | 2 |
| v1.0.7 | 2026-06-17 | 1 |
| v1.0.8 | 2026-06-17 | 7 |
| v1.0.9 | 2026-06-19 | 2 |
| v1.0.10 | 2026-06-19 | 0 |
| **总计（终身）** | | **41** |

- **7d 下载 (6/12~6/19)**: 20
- **3d 下载 (6/16~6/19, awesome-tuis merge 后)**: 12
- **换算均值**: ~4 下载/天（post-awesome-tuis）

**按平台分布**：

| 平台 | 终身下载 | % |
|---|---:|---:|
| **macOS ARM64 (M1+)** | 20 | **49%** |
| Linux x64 | 15 | 37% |
| Windows x64 | 3 | 7% |
| Linux ARM64 | 1 | 2% |
| macOS Intel | 1 | 2% |
| Windows ARM64 | 1 | 2% |

> 🔑 **M1+ Mac 是主要用户**，所有 demo/截图/教程都该默认 Mac 场景。

**GitHub 仓库流量（仅供参考，不是真实装机）**：

| 指标 | 数值 |
|---:|---:|
| Views (14d) | 271 |
| Unique visitors (14d) | 45 |
| Clones (14d, ⚠️ dev/bot) | 1,312 |
| Unique cloners (14d, ⚠️ dev/bot) | 441 |
| Stars (total) | 10 |

**Top referrers (14d)**：

| Referrer | Count | Uniques |
|---|---:|---:|
| github.com | 113 | 24 |
| **Google** | **5** | **2** |
| Bing | 2 | 2 |
| DuckDuckGo | 1 | 1 |
| reddit.com | 1 | 1 |

**Top pages (14d)**：

| Path | Count | Uniques |
|---|---:|---:|
| `/sollawen/microNeo` (Overview) | 133 | 43 |
| `/sollawen/microNeo/releases` | 18 | 3 |
| `/sollawen/microNeo/actions` | 14 | 1 |
| `/sollawen/microNeo/graphs/traffic` | 14 | 1 |
| `/sollawen/microNeo/pulse` | 12 | 1 |
| `/sollawen/microNeo/tree/dev103` | 8 | 1 |
| `/sollawen/microNeo/stargazers` | 7 | 2 |
| `/sollawen/microNeo/tree/master` | 5 | 5 |
| `/sollawen/microneo` (typo) | 5 | 1 |
| `/sollawen/microNeo/blob/master/assets/microneo-demo2.png` | 4 | 4 |

**Google 索引**: 未检查（手动 PAA 检查随后补）
**PAA 命中**: 0/5（手动检查见后续 entry）

**Analysis**:
- **真正装机数 = 41 / 终身 = ~15 个/周**。MKT-plan 之前估的 10-15/14 天偏低了，**实际是 ~20/7d**
- 当前流量 100% 依赖 awesome-tuis 长尾（间接通过 github.com referrer 的 24 uniques 中大部分来自 awesome-tuis 链接点击）
- Google referrer 5/2 = 来自 Google 的访问极少（**SEO 改动前的状态**）
- demo 图被 4 个独立访客访问 —— 已经有 Google 图片搜索流量
- `/sollawen/microneo`（小写 n）有 5 次访问，说明有人通过这个变体名搜到
- **转化漏斗**: Views 271 → 装机 41 = **15% visitor→install 转化率**（行业 2-5%，我们 3-7x 优秀）

**Action items**:
- [ ] 下周日（6/26）跑 W1 第一次 weekly check
- [ ] 6/26 手动查 5 个目标关键词的 PAA 状态
- [ ] 6/26 跑 `site:github.com/sollawen/microNeo` 看 Google 索引
- [ ] 6/26 跑 Measurement Playbook 第 1 节得到 binary download 对比数据

---
