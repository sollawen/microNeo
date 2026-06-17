# Linux 测试手册 — `extract-changelog.sh`

**目的**：在 Linux 上用 GNU awk 跑同样测试用例，验证 workflow 在 GitHub runner（ubuntu-latest）上的行为。

**背景**：mac 默认的 BSD awk 在某些 regex 行为上跟 GNU awk 有差异。我已经把脚本改成"零 regex 匹配"写法（用 `substr + ==` 做字符串比较），理论上两边行为应该一致 —— 但需要 Linux 实测确认。

**前置**：在 Linux 机器上 `cd` 到项目根目录，假设已经 `git pull` 到最新。

---

## Step 0：同步代码

```bash
cd /path/to/microNeo
git pull origin main
git status   # 确认 working tree clean
```

应该看到以下两个文件**已更新**：

- `tools/extract-changelog.sh`（脚本）
- `CHANGELOG.md`（改成 Keep a Changelog 风格 + v1.0.6 还在 [Unreleased] 段）

确认脚本有可执行权限：

```bash
ls -la tools/extract-changelog.sh
# 期望: -rwxr-xr-x (有 x 权限)
```

如果没有：

```bash
chmod +x tools/extract-changelog.sh
```

确认 awk 是 GNU 版（Linux 默认就是）：

```bash
awk --version 2>&1 | head -1
# 期望: GNU Awk 5.x
```

---

## Step 1：8 个测试用例

把下面整块命令贴到 Linux 终端一次性跑完：

```bash
cd /path/to/microNeo

PASS=0; FAIL=0
check() {
  local name="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    echo "✅ $name: $actual"
    PASS=$((PASS+1))
  else
    echo "❌ $name: expected '$expected', got '$actual'"
    FAIL=$((FAIL+1))
  fi
}

# --- Test 1: v1.0.6 (当前在 [Unreleased]，应失败，提示 available versions) ---
# 这一条是预期失败：v1.0.6 还没发，CHANGELOG 里就是 [Unreleased]
# 用来验证：发版前忘记改名，workflow 会抽错段（这就是我们设计想要的"安全失败"）
cp CHANGELOG.md /tmp/CHANGELOG.md.bak
tools/extract-changelog.sh 1.0.6 /tmp/test-v1.0.6.md
EXITCODE=$?
check "T1.v1.0.6-not-yet-released" "1" "$EXITCODE"
cp /tmp/CHANGELOG.md.bak CHANGELOG.md

# --- Test 2: v1.0.5 (已发布段) ---
tools/extract-changelog.sh 1.0.5 /tmp/test-v1.0.5.md
EXITCODE=$?
check "T2.v1.0.5-exit" "0" "$EXITCODE"
# 期望 13 行
check "T2.v1.0.5-lines" "13" "$(wc -l < /tmp/test-v1.0.5.md)"
# 期望包含关键内容
grep -q "End-of-file panic" /tmp/test-v1.0.5.md && check "T2.content-panic" "yes" "yes" || check "T2.content-panic" "yes" "no"
grep -q "view portRowmap" /tmp/test-v1.0.5.md && check "T2.content-viewport" "yes" "yes" || check "T2.content-viewport" "yes" "no"
grep -q "microNeo-debug.log" /tmp/test-v1.0.5.md && check "T2.content-debug" "yes" "yes" || check "T2.content-debug" "yes" "no"

# --- Test 3: v1.0.4 ---
tools/extract-changelog.sh 1.0.4 /tmp/test-v1.0.4.md
EXITCODE=$?
check "T3.v1.0.4-exit" "0" "$EXITCODE"
check "T3.v1.0.4-lines" "10" "$(wc -l < /tmp/test-v1.0.4.md)"
grep -q "bufferLineToScreenOffset" /tmp/test-v1.0.4.md && check "T3.content" "yes" "yes" || check "T3.content" "yes" "no"

# --- Test 4: v1.0.3 ---
tools/extract-changelog.sh 1.0.3 /tmp/test-v1.0.3.md
EXITCODE=$?
check "T4.v1.0.3-exit" "0" "$EXITCODE"
check "T4.v1.0.3-lines" "6" "$(wc -l < /tmp/test-v1.0.3.md)"
grep -q "Inline-code background" /tmp/test-v1.0.3.md && check "T4.content" "yes" "yes" || check "T4.content" "yes" "no"

# --- Test 5: v1.0.6 (临时改名测完整内容) ---
sed -i 's/## \[Unreleased\]/## [1.0.6]/' CHANGELOG.md
tools/extract-changelog.sh 1.0.6 /tmp/test-v1.0.6.md
EXITCODE=$?
check "T5.v1.0.6-renamed-exit" "0" "$EXITCODE"
check "T5.v1.0.6-renamed-lines" "20" "$(wc -l < /tmp/test-v1.0.6.md)"
grep -q "LineToScreenRow" /tmp/test-v1.0.6.md && check "T5.content-line" "yes" "yes" || check "T5.content-line" "yes" "no"
grep -q "MDRender" /tmp/test-v1.0.6.md && check "T5.content-mdconfig" "yes" "yes" || check "T5.content-mdconfig" "yes" "no"
grep -q "decoration row" /tmp/test-v1.0.6.md && check "T5.content-decoration" "yes" "yes" || check "T5.content-decoration" "yes" "no"
cp /tmp/CHANGELOG.md.bak CHANGELOG.md

# --- Test 6: v1.0.2 (不在 CHANGELOG) ---
tools/extract-changelog.sh 1.0.2 /tmp/test-v1.0.2.md
EXITCODE=$?
check "T6.v1.0.2-missing" "1" "$EXITCODE"
# 错误信息应包含 [1.0.5]（之前看到显示缺 ]，已修复）
grep -q "## \[1.0.5\]" /tmp/test-v1.0.2.md
# 错误是去 stderr 的，不会写到输出文件。所以检查 stderr:
STDERR=$(tools/extract-changelog.sh 1.0.2 /tmp/test-v1.0.2.md 2>&1 >/dev/null)
echo "$STDERR" | grep -q "## \[1.0.5\]" && check "T6.error-shows-versions" "yes" "yes" || check "T6.error-shows-versions" "yes" "no"

# --- Test 7: v9.9.9 (不存在) ---
tools/extract-changelog.sh 9.9.9 /tmp/test-9.9.9.md
EXITCODE=$?
check "T7.v9.9.9-not-found" "1" "$EXITCODE"

# --- Test 8: 空参数 ---
tools/extract-changelog.sh 2>/dev/null
EXITCODE=$?
check "T8.empty-arg" "1" "$EXITCODE"

# --- Test 9: 缺第二个参数 ---
tools/extract-changelog.sh 1.0.6 2>/dev/null
EXITCODE=$?
check "T9.missing-arg" "1" "$EXITCODE"

echo ""
echo "=========================="
echo "PASS: $PASS  FAIL: $FAIL"
echo "=========================="
[ $FAIL -eq 0 ] && echo "🎉 全部通过，可以回 mac 改 workflow 了" || echo "❌ 有失败项，把输出贴回给 AI"
```

---

## Step 2：输出预期

**全部通过**时输出应该是：

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

## Step 3：失败时怎么办

把整段命令输出（连同错误）贴回给我。**常见可能**：

| 失败模式 | 可能原因 | 我会怎么修 |
|---|---|---|
| 任意 `T2/T3/T4/T5 exit` 不为 0 | awk 语法在 GNU 上崩了 | 改脚本 |
| 行数不对 | 段落切分错位 | 改 awk pattern |
| content 检查失败 | 版本号匹配错 | 改 awk 匹配逻辑 |
| `T6.error-shows-versions` no | grep 输出又被转义吃了 | 改用 awk 格式化 |

CHANGELOG.md 应该已经被 Step 5 的 `cp /tmp/CHANGELOG.md.bak CHANGELOG.md` 还原成 `[Unreleased]` 了。**如果看到还是 `[1.0.6]`，手动改回来**：

```bash
sed -i 's/## \[1.0.6\]/## [Unreleased]/' CHANGELOG.md
git diff CHANGELOG.md   # 确认干净
```

---

## Step 4：通过后回 mac 的工作

回到 mac 后我会：

1. 改 `.github/workflows/release.yaml`（加 Extract step + `body_path`）
2. 给你看 `git diff` 让你审
3. `git add` + `git commit`
4. 改 `CHANGELOG.md`：`## [Unreleased]` → `## [1.0.6] - 2026-06-17`
5. `git commit` "docs: release v1.0.6"
6. `git push origin main`
7. `git tag v1.0.6 && git push origin v1.0.6`
8. 看 GitHub Actions 跑完
9. 验证 release notes 是 v1.0.6 段内容
