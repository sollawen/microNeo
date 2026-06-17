# Linux 测试报告 — `extract-changelog.sh` & `release.yaml`

**日期**：2026-06-17
**执行者**：AI（microNeo）
**环境**：

- OS: Linux 6.8.0-111-generic (Ubuntu)
- awk: GNU Awk 5.2.1
- 仓库分支: `master`
- HEAD: `32791ddd feat(release): add extract-changelog script for automated release notes`

---

## 1. 测试执行

### 1.1 环境前提

| 项 | 状态 |
|---|---|
| `tools/extract-changelog.sh` 存在 | ✅ |
| 脚本可执行权限 (`-rwxr-xr-x`) | ✅ |
| `CHANGELOG.md` 顶层是 `## [Unreleased]` | ✅ |
| GNU Awk 5.x | ✅ (5.2.1) |
| working tree clean（测试前） | ✅ |

### 1.2 测试结果

**最终：22 / 22 全部通过** ✅

```
✅ T1.v1.0.6-not-yet-released: 1
✅ T2.v1.0.5-exit: 0
✅ T2.v1.0.5-lines: 13
✅ T2.content-panic: yes
✅ T2.content-viewport: yes
✅ T2.content-debug: yes
✅ T3.v1.0.4-exit: 0
✅ T3.v1.0.4-lines: 10
✅ T3.content: yes
✅ T4.v1.0.3-exit: 0
✅ T4.v1.0.3-lines: 6
✅ T4.content: yes
✅ T5.v1.0.6-renamed-exit: 0
✅ T5.v1.0.6-renamed-lines: 20
✅ T5.content-line: yes
✅ T5.content-mdconfig: yes
✅ T5.content-decoration: yes
✅ T6.v1.0.2-missing: 1
✅ T6.error-shows-versions: yes
✅ T7.v9.9.9-not-found: 1
✅ T8.empty-arg: 1
✅ T9.missing-arg: 1

==========================
PASS: 22  FAIL: 0
==========================
🎉 全部通过，可以回 mac 改 workflow 了
```

---

## 2. 发现并修复

### 2.1 测试计划自身有 1 个 typo

**问题**：`docs/test-plan-linux.md` 的 T2.content-viewport 断言期望子串 `"view portRowmap"`（带空格），但 `CHANGELOG.md` 实际内容是 `"viewportRowmap"`（一个词），所以首次跑会失败。

**根因**：测试计划里的 grep pattern 复制时多打了空格。

**修复**：

```diff
-grep -q "view portRowmap" /tmp/test-v1.0.5.md && check "T2.content-viewport" "yes" "yes" || check "T2.content-viewport" "yes" "no"
+grep -q "viewportRowmap" /tmp/test-v1.0.5.md && check "T2.content-viewport" "yes" "yes" || check "T2.content-viewport" "yes" "no"
```

**确认**：修复后重跑全套，22 / 22 通过。脚本本身（`tools/extract-changelog.sh`）无需改动 —— 验证 BSD/GNU awk 行为一致的目标已达成。

### 2.2 `extract-changelog.sh` 行为总结

| 场景 | 退出码 | stderr | 输出文件 |
|---|---|---|---|
| 版本存在（已发布段） | 0 | 静默 | 段内容 |
| 版本在 `[Unreleased]` | 1 | `::error::No CHANGELOG.md entry found` + 列出已存在版本 | 空 |
| 版本完全不存在 | 1 | 同上 | 空 |
| 空参数 | 1 | `usage: ...` （来自 bash `${1:?}` 展开） | — |
| 缺第二个参数 | 1 | 同上 | — |
| `CHANGELOG.md` 不存在 | 1 | `::error::CHANGELOG.md not found` | — |

`::error::` 前缀是 GitHub Actions annotation 语法，会在 Actions UI 醒目显示。

---

## 3. workflow 改造

### 3.1 改动 diff

**文件**：`.github/workflows/release.yaml`

```diff
@@ -25,12 +25,19 @@ jobs:
     - name: Build
       run: tools/cross-compile.sh

+    - name: Extract changelog
+      run: |
+        # Tag 形如 v1.0.6，脚本要的是 1.0.6
+        VERSION="${GITHUB_REF_NAME#v}"
+        tools/extract-changelog.sh "$VERSION" /tmp/release-notes.md
+
     - name: Publish
       uses: softprops/action-gh-release@v2
       env:
         GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
       with:
         files: binaries/*
+        body_path: /tmp/release-notes.md
         generate_release_notes: true
```

### 3.2 设计要点

1. **Extract step 位置**：放在 Build 之后、Publish 之前。理由：Build 失败时本就不该 Publish；Extract 依赖 Checkout 已经把 `CHANGELOG.md` 拉下来（`fetch-depth: 0` + `fetch-tags: true` 确保完整历史）。

2. **`${GITHUB_REF_NAME#v}`**：剥离 tag 前缀 `v`。GitHub Actions 在 tag push 时 `GITHUB_REF_NAME` 是 `v1.0.6` 这种形式，脚本要的是 `1.0.6`。

3. **`body_path` + `generate_release_notes: true` 共存**：`softprops/action-gh-release@v2` 文档明确 `body` / `body_path` 优先，`generate_release_notes` 会被覆盖 —— 所以留 `generate_release_notes: true` 是冗余但无害，可保留作 fallback，也可后续清理。**当前选择保留**，等真发版后看实际效果再决定。

4. **失败传播**：脚本非零退出 → step 失败 → workflow 失败 → 不会触发 Publish。这正是"安全失败"设计：发版前忘了把 `[Unreleased]` 改成 `[1.0.6]`，整个 release 就跑不起来，强迫人工复核。

### 3.3 验证

**YAML 语法**：`python3 yaml.safe_load` 解析通过。

**Steps 顺序**：

```
- Checkout
- Setup Go
- Build
- Extract changelog   ← 新增
- Publish
- Cleanup
```

**端到端模拟**（用 `GITHUB_REF_NAME=v1.0.6` 喂给当前 `[Unreleased]` 状态的 CHANGELOG）：

```
::error::No CHANGELOG.md entry found for v1.0.6
::error::Available versions:
## [1.0.5]
## [1.0.4]
## [1.0.3]
Actual script exit code: 1
```

符合"安全失败"预期。

---

## 4. 改动清单

| 文件 | 行数 | 性质 |
|---|---|---|
| `.github/workflows/release.yaml` | +7 | 自动化发布说明 |
| `docs/test-plan-linux.md` | +1 / -1 | 测试计划 typo 修正 |

**未改动**：

- `tools/extract-changelog.sh` —— 测试通过，无需改
- `CHANGELOG.md` —— 保持 `## [Unreleased]`，等人工改 `[1.0.6]` 再单独 commit

`git status` 干净（除上述两个文件外）。

---

## 5. 待确认 / 下一步

### 5.1 待用户决定

1. **commit 策略**（选其一）：
   - A. 两个文件一起：`feat(ci): automate release notes + fix test plan typo`
   - B. 分开两次：workflow 一次、docs 一次

2. **分支名**：当前在 `master`，测试计划写的 `git push origin main`。是否用 `master`？

3. **CHANGELOG 改名时机**：是 (a) 跟 workflow 一起 push 完再改，还是 (b) workflow merge 之后再单独 commit 改名？
   - 建议 (b)：先 merge workflow，让 main/master 跑过 CI 验证 Extract step 不会炸，再改 CHANGELOG 触发 release。
   - 风险 (a)：如果 workflow 有 bug 且需要 fast-fix，会跟 CHANGELOG 改动混在一起。

### 5.2 完成 v1.0.6 发布的剩余步骤（Step 4 后续）

按 `docs/test-plan-linux.md` 的 Step 4：

1. 决定 commit 策略 → commit workflow 改动 → push
2. 改 `CHANGELOG.md`：`## [Unreleased]` → `## [1.0.6] - 2026-06-17`
3. `git commit` "docs: release v1.0.6"
4. `git push origin master`（或 main）
5. `git tag v1.0.6 && git push origin v1.0.6`
6. 在 GitHub 上看 Actions 跑完
7. 验证 release notes 是 v1.0.6 段内容

---

## 6. 结论

- ✅ GNU awk 5.2.1 下 `extract-changelog.sh` 行为完全符合预期
- ✅ workflow 改造 diff 干净、YAML 有效、端到端模拟通过
- ✅ "安全失败"机制工作正常（忘改 `[Unreleased]` 就跑不通）
- 📋 两个文件待 commit，三个决策点待用户确认
