# Release Notes Pipeline — Plan

**目标**：push tag → Action 自动从 `CHANGELOG.md` 抽取对应 version 段落，写入 GitHub Release notes。无需事后手动编辑。

---

## 为什么选这个方案

项目里 `CHANGELOG.md` 已经是手工维护的高质量总结（v1.0.5 / v1.0.6 段都是手写），内容比 commit 自动拼接更适合 release notes：

- **跨 commit 总结**（例：「Split the render pipeline into four single-responsibility stages」是 5 个 commit 综合描述，commit 列表会割裂表达）
- **维护纪律已有**（每次发版前你都在 update CHANGELOG）
- **零迁移成本**（commit 风格不需要改）

不选 conventional commit 自动分类的原因：v1.0.4 那段手写 notes 是项目历史上唯一"有内容"的 release，已经证明了手写价值，自动化反而退步。

---

## 改动清单

### 新增 1 个文件

**`tools/extract-changelog.sh`** — 从 `CHANGELOG.md` 抽出指定 version 段落到临时文件

```bash
#!/usr/bin/env bash
# 用法: extract-changelog.sh <version-without-v> <output-file>
# 例:   extract-changelog.sh 1.0.6 /tmp/release-notes.md
# 行为: 找 "## [1.0.6]" heading，提取到下一个 "## [X.Y.Z]" 之前所有内容
# 失败: 找不到对应段 → 退出码 1，触发 workflow 失败
set -euo pipefail

VERSION="${1:?usage: extract-changelog.sh <version> <output-file>}"
OUT="${2:?usage: extract-changelog.sh <version> <output-file>}"

if [ ! -f CHANGELOG.md ]; then
  echo "::error::CHANGELOG.md not found at repo root" >&2
  exit 1
fi

awk -v target="## \\[${VERSION}\\]" '
  $0 ~ target { found=1; next }
  /^## \[[0-9]+\.[0-9]+\.[0-9]+/ { found=0 }
  found { print }
' CHANGELOG.md > "$OUT"

if [ ! -s "$OUT" ]; then
  echo "::error::No CHANGELOG.md entry found for v${VERSION}" >&2
  echo "::error::Available versions:" >&2
  grep -oE '## \[[0-9]+\.[0-9]+\.[0-9]+' CHANGELOG.md >&2 || true
  exit 1
fi
```

**awk 匹配规则说明**：

- `target="## \\[${VERSION}\\]"` — 字面匹配 `## [1.0.6]`（方括号在 awk regex 里是字符类，所以用 `\\[` `\\]` 转义）
- `/^## \[[0-9]+\.[0-9]+\.[0-9]+/` — 终止条件：遇到下一个 version heading 就停
- `[Unreleased]` 不会被 `^## \[[0-9]+\.[0-9]+\.[0-9]+` 匹配（因为不是数字开头），所以**只能靠 `target` 显式触发**——这意味着 `[Unreleased]` 段永远不会被错误地抽到某个具体版本的 release 里

### 修改 1 个文件

**`.github/workflows/release.yaml`** — 替换 Publish step，新增 Extract step

```yaml
name: Release builds
on:
  push:
    tags:
      - 'v*.*.*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
    - name: Checkout
      uses: actions/checkout@v4
      with:
        fetch-depth: 0
        fetch-tags: true

    - name: Setup Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.23'

    - name: Build
      run: tools/cross-compile.sh

    - name: Extract release notes from CHANGELOG.md
      id: notes
      run: tools/extract-changelog.sh "${GITHUB_REF_NAME#v}" /tmp/release-notes.md

    - name: Publish
      uses: softprops/action-gh-release@v2
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      with:
        files: binaries/*
        body_path: /tmp/release-notes.md
        # 不再需要 generate_release_notes: true，body_path 完全覆盖

    - name: Cleanup
      run: rm -rf binaries
```

---

## 行为详解

### push tag 后的完整流程

1. **Checkout** — 拉取代码（带所有 tag）
2. **Setup Go** — 装 Go 1.23
3. **Build** — `tools/cross-compile.sh` 交叉编译到 `binaries/`
4. **Extract** — 跑 `extract-changelog.sh`：
   - `GITHUB_REF_NAME` 是 `v1.0.6` → 去掉 `v` 前缀传 `1.0.6`
   - awk 在 `CHANGELOG.md` 里找 `## [1.0.6]`
   - 抽取该行以下、所有 `## [X.Y.Z]` 之前的内容
   - 写入 `/tmp/release-notes.md`
   - **找不到对应段 → 整个 workflow 失败**（防止漏写 changelog 还能发版）
5. **Publish** — `softprops/action-gh-release` 读取 `/tmp/release-notes.md` 作为 release body
6. **Cleanup** — 删 `binaries/`

### v1.0.6 发布后效果

Release page 会显示：

> **v1.0.6**
>
> ### Added
> - Exported `LineToScreenRow` / `ScreenRowToLine` for screen↔buffer coordinate conversion.
>
> ### Changed
> - Rewrote the MD rendering pipeline around a single `screenBuffer` data source, replacing the v1.0.5 dual-write viewportRowmap...
> - Split the render pipeline into four single-responsibility stages...
> - Improved table rendering and added display-package unit tests.
>
> ### Fixed
> - Cursor no longer drifts or disappears when navigating across table/code-block segments...
> - ... (CHANGELOG.md 里的全部 v1.0.6 条目)
>
> ### Removed
> - The `MDRender` and `MDRenderIdle` config options (rendering is now unconditional when MD is enabled).
>
> **Full Changelog**: https://github.com/sollawen/microNeo/compare/v1.0.5...v1.0.6

后面那行 "Full Changelog" 是 GitHub Release 页面自己加的，不需要我们写。

---

## 边界情况

| 情况 | 行为 |
|---|---|
| `CHANGELOG.md` 缺 v1.0.6 段 | workflow 失败，提示 available versions |
| `[Unreleased]` 段被推到 v1.0.6 tag | Action 不会失败（awk 抽出 [Unreleased] 段），但 release notes 里会出现 "[Unreleased]" 字样——**非硬错，记得发版前改名** |
| 段内有空行 | 保留空行（markdown 渲染换段需要） |
| 段尾多余空行 | awk 输出末尾有 `\n`，`softprops` 会处理 |
| 中英混合 / emoji | 原样保留 |

---

## 发布 v1.0.6 时的额外步骤

CHANGELOG 现在用 Keep a Changelog 风格（`## [X.Y.Z] - YYYY-MM-DD`），v1.0.6 还在 `[Unreleased]` 段下。

发版前要把 `[Unreleased]` → `[1.0.6] - YYYY-MM-DD`：

```bash
# 1. 改 CHANGELOG.md：把 ## [Unreleased] 改成 ## [1.0.6] - $(date +%Y-%m-%d)
# 2. 提交
git add CHANGELOG.md
git commit -m "docs: release v1.0.6"
# 3. 打 tag push
git tag v1.0.6
git push origin main --tags
```

> 如果忘了改 `[Unreleased]`，Action 不会失败——awk 会把 `[Unreleased]` 段原样发出去，release notes 里会出现 "[Unreleased]" 字样。**不是硬错，但记得改**。

---

## 后续操作总览

1. 给 `tools/extract-changelog.sh` 加可执行权限：`chmod +x tools/extract-changelog.sh`
2. 修改 `.github/workflows/release.yaml`（按上面 diff）
3. 按上面"发版前"步骤改 CHANGELOG.md 的 `[Unreleased]` → `[1.0.6] - 日期`
4. 提交并 push 到 main
5. push tag v1.0.6 → Action 自动从 CHANGELOG 抽段发 release

回退方案：如果某次临时想自定义 notes 文本（比如紧急 hotfix），可以：
- 直接 push tag → workflow 失败
- 手动 `gh release create v1.0.6 --notes "临时文本"` 跳过 Action
- 或者临时在 CHANGELOG.md 写一个特殊段落

---

## 为什么不选其他方案

| 方案 | 否决理由 |
|---|---|
| **A. 现状**（push → 自动空 notes → 手动 `gh release edit`） | 每次都要手动补，已证明会忘（v1.0.5 漏了） |
| **B. conventional commit 自动分类** | changelog 里有跨 commit 总结，commit 拼接会割裂表达；中文 commit 标题难看 |
| **D. Draft release → 手动 publish** | 多一步操作，没解决自动化问题 |

---

## 本地测试计划

**为什么必须本地测**：workflow 在 Linux runner 上跑（GNU awk），但你本机是 mac（BSD awk）。BSD 和 GNU 的 awk 在字符类、regex 转义上行为有差异，本地测了不等于 runner 上 100% 正确——但至少能抓 90% 的语法错。

### 测试方法 1：mac 本地快速测（用 BSD awk）

```bash
# 1. 落地脚本后赋可执行权限
chmod +x tools/extract-changelog.sh

# 2. 测 v1.0.6（当前在 [Unreleased] 段）
tools/extract-changelog.sh 1.0.6 /tmp/test-v1.0.6.md
cat /tmp/test-v1.0.6.md

# 3. 测 v1.0.5（已发布版本）
tools/extract-changelog.sh 1.0.5 /tmp/test-v1.0.5.md
cat /tmp/test-v1.0.5.md

# 4. 测不存在的版本（应失败）
tools/extract-changelog.sh 9.9.9 /tmp/test-fail.md
echo "exit code: $?"   # 期望 1
```

### 测试方法 2：用 Docker 跑 GNU awk（100% 还原 runner）

```bash
# 起一个 ubuntu 容器，把项目目录挂进去
docker run --rm -v "$PWD":/workspace -w /workspace ubuntu:24.04 bash -c '
  apt-get update -qq && apt-get install -y -qq gawk
  chmod +x tools/extract-changelog.sh
  tools/extract-changelog.sh 1.0.6 /tmp/test-v1.0.6.md
  echo "=== content ==="
  cat /tmp/test-v1.0.6.md
'
```

### 验收标准

| 测试 | 期望输出 |
|---|---|
| `extract-changelog.sh 1.0.6` | 包含 Added/Changed/Fixed/Removed 四节，6 条 Fix + 3 条 Changed + 1 条 Added + 1 条 Removed |
| `extract-changelog.sh 1.0.5` | 4 条 Changed + 1 条 Fixed + 1 条 Removed |
| `extract-changelog.sh 1.0.4` | 1 Added + 1 Changed + 1 Fixed |
| `extract-changelog.sh 1.0.3` | 1 Changed + 1 Fixed |
| `extract-changelog.sh 9.9.9` | exit code 1，stderr 含 "No CHANGELOG.md entry found" |
| `extract-changelog.sh ""` | exit code 1，stderr 含 "usage" |

### 已知 mac/BSD awk 陷阱

1. **`\<` / `\>` 单词边界**：BSD 不支持，避免用
2. **`\d` 数字简写**：BSD 不支持，用 `[0-9]`
3. **`-v target="..."` 中的 regex 字符**：BSD 在 `-v` 注入的变量里 `\[` `\]` 可能解释不同

我们的脚本里第三点会有风险，**所以强烈建议至少跑一次 Docker 测**确认。
