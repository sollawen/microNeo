# microNeo 文档站点方案（mkdocs-material）

> 状态：**讨论中**
> 创建日期：2026-06-22
> 目标：用 mkdocs-material 搭建 GitHub Pages 文档站

---

## 1. 目标

- 为 microNeo 提供独立的文档站点，托管在 GitHub Pages
- 站内先放一个类似 README 的首页，后续按需扩展
- 中英双语，英文为主，用户可手动切换

---

## 2. 最终目录结构

```
microNeo/
├── mkdocs.yml                         # ⭐ mkdocs 主配置（根目录）
├── docs/                              # 现有开发文档（不动）
│   ├── AGENTS.md
│   ├── introduce.md
│   ├── 整体架构说明.md
│   ├── RenderedSegment数据结构.md
│   ├── ... (其他开发笔记保留原样)
│   └── mkdocs-website-plan.md         # 本文档
├── docs/website/                      # ⭐ 站点文档入口
│   ├── en/                            # 英文（默认）
│   │   └── index.md                   # 英文首页
│   └── zh/                            # 中文
│       └── index.md                   # 中文首页
├── README.md                          # 仓库主页（保留）
└── .github/
    └── workflows/
        └── deploy-docs.yml            # ⭐ 部署 workflow
```

---

## 3. 关键决策

| 项 | 选择 | 原因 |
|---|---|---|
| 工具 | mkdocs-material | 简单、双语友好、心智负担小 |
| 子目录 | `docs/website/` | 跟开发文档区分 |
| 配置位置 | 根目录 `mkdocs.yml` | GitHub Actions 标准路径 |
| 双语方式 | 双目录 (`en/`、`zh/`) | 简单直接，无需插件 |
| 初始内容 | 1 个英文首页 + 1 个中文首页 | 先把骨架跑通 |
| 构建产物 | `.gitignore` 加 `/site/` | 防止本地 `mkdocs build` 产物误提交；部署走 GitHub Actions 云端构建，不受影响 |
| 内容来源 | 基于现有 README.md 改写 | 已经有现成材料 |
| 默认语言 | 英文 | 跟 README 策略一致 |

---

## 4. 双语切换实现

不用任何第三方插件，靠 mkdocs-material 内置的 `extra.alternate` 功能：

```yaml
# mkdocs.yml
extra:
  alternate:
    - name: English
      link: /microNeo/en/
      lang: en
    - name: 中文
      link: /microNeo/zh/
      lang: zh
```

效果：页面右上角自动出现 🌐 语言切换按钮，点击跳转到对应语言的首页。

---

## 5. mkdocs.yml 完整配置（草稿）

```yaml
site_name: microNeo
site_description: Terminal Markdown Editor — render and edit in the same window
site_url: https://sollawen.github.io/microNeo/
repo_url: https://github.com/sollawen/microNeo
repo_name: sollawen/microNeo
edit_uri: edit/main/docs/website/

theme:
  name: material
  language: en
  features:
    - navigation.instant
    - navigation.tracking
    - navigation.top
    - search.highlight
    - search.suggest
    - content.code.copy
    - content.tabs.link
    - content.action.edit
  palette:
    - media: "(prefers-color-scheme: light)"
      scheme: default
      primary: indigo
      accent: indigo
      toggle:
        icon: material/brightness-7
        name: Switch to dark mode
    - media: "(prefers-color-scheme: dark)"
      scheme: slate
      primary: indigo
      accent: indigo
      toggle:
        icon: material/brightness-4
        name: Switch to light mode

markdown_extensions:
  - admonition
  - attr_list
  - def_list
  - footnotes
  - md_in_html
  - tables
  - toc:
      permalink: true
  - pymdownx.details
  - pymdownx.superfences
  - pymdownx.tabbed:
      alternate_style: true

plugins:
  - search

extra:
  alternate:
    - name: English
      link: /microNeo/en/
      lang: en
    - name: 中文
      link: /microNeo/zh/
      lang: zh
  social:
    - icon: fontawesome/brands/github
      link: https://github.com/sollawen/microNeo

nav:
  - Home: en/index.md
  - 首页: zh/index.md
```

---

## 6. 初始文件清单

最小可用集（先让站点跑起来）：

| 路径 | 内容 | 来源 |
|------|------|------|
| `mkdocs.yml` | 主配置 | 全新写 |
| `docs/website/en/index.md` | 英文首页 | 基于 `README.md`（英文段）改写 |
| `docs/website/zh/index.md` | 中文首页 | 基于 `README.md`（中文段）改写 |
| `.github/workflows/deploy-docs.yml` | 部署 workflow | 全新写 |
| `.gitignore` | 追加 `/site/` 一行 | 忽略本地 `mkdocs build` 产物 |

后续可扩展：

| 路径 | 内容 |
|------|------|
| `docs/website/en/getting-started.md` | 安装与快速开始 |
| `docs/website/en/guide/basic-editing.md` | 基本编辑 |
| `docs/website/en/guide/markdown-rendering.md` | Markdown 渲染说明 |
| `docs/website/en/guide/configuration.md` | 配置说明 |
| `docs/website/en/faq.md` | FAQ（基于 README 已有 FAQ） |
| `docs/website/en/changelog.md` | 变更日志（基于 CHANGELOG.md） |
| `docs/website/zh/...` | 对应中文版本 |

---

## 7. 部署方案

### GitHub Actions workflow

```yaml
# .github/workflows/deploy-docs.yml
name: Deploy Docs
on:
  push:
    branches: [ main ]
    paths:
      - 'mkdocs.yml'
      - 'docs/website/**'
      - '.github/workflows/deploy-docs.yml'
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: pages
  cancel-in-progress: false

jobs:
  deploy:
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-python@v5
        with:
          python-version: 3.x

      - run: pip install mkdocs mkdocs-material

      - run: mkdocs gh-deploy --force --clean

      - name: Setup Pages
        uses: actions/configure-pages@v4

      - name: Upload artifact
        uses: actions/upload-pages-artifact@v3
        with:
          path: site

      - id: deployment
        uses: actions/deploy-pages@v4
```

### 仓库配置

1. Settings → Pages → Source: **GitHub Actions**
2. Settings → Actions → General → Workflow permissions: **Read and write permissions**
3. 首次部署会自动创建 `gh-pages` 分支

### 触发规则

- 修改 `mkdocs.yml`、`docs/website/**` 或本 workflow 时自动部署
- 其他改动（如修改 README、修改 Go 代码）不触发

---

## 8. 执行步骤（待用户批准后）

> ⚠️ 当前处于 PLAN 模式，需要用户切换到 EDIT 模式后才能动手。

1. 安装 mkdocs-material（本地预览用）：
   ```bash
   pip install mkdocs mkdocs-material
   ```
2. 创建 `mkdocs.yml`（根目录）
3. 创建 `docs/website/en/index.md`（基于 README 改写）
4. 创建 `docs/website/zh/index.md`（基于 README 改写）
5. 创建 `.github/workflows/deploy-docs.yml`
6. 修改 `.gitignore`，追加：
   ```gitignore
   # mkdocs 本地构建产物（mkdocs build 落到仓库根的静态站点目录）
   /site/
   ```
   > 注：`site/` 是 mkdocs 的默认构建输出目录（硬编码，不可配置）。本地 `mkdocs build` 会落盘，CI 的 `mkdocs gh-deploy` 不会。`/site/` 入 `.gitignore` 后，网站部署不受任何影响 —— 部署走的是 GitHub Actions 云端构建 + `upload-pages-artifact`，产物直接交给 Pages 服务，不经过 git。
7. 本地预览验证：`mkdocs serve`，浏览器打开 `http://127.0.0.1:8000`
8. 推到 GitHub，等待 Actions 自动部署
9. 访问 `https://sollawen.github.io/microNeo/` 验证

---

## 9. 待确认事项

- [ ] 子目录名 `docs/website/` 是否 OK
- [ ] 主题色调 `indigo` 是否合适（或想换别的？）
- [ ] 是否需要在首页加 logo / favicon（用 `assets/` 里现有的 SVG）
- [ ] GitHub Pages URL 路径是否需要自定义（现在是 `https://sollawen.github.io/microNeo/`）

---

## 10. 后续扩展路线图

阶段一（当前）：骨架 + 双语首页
阶段二：FAQ、changelog
阶段三：使用指南（基本编辑、Markdown 渲染、配置）
阶段四：架构文档（从 `整体架构说明.md` 精选）
阶段五：搜索优化、SEO、徽章