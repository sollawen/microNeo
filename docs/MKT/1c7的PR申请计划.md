# chinese-independent-developer 主版面申请方案

## 一、申请内容

### 仓库
https://github.com/sollawen/microNeo

### 详情页（`[更多介绍]` 链接目标）
https://sollawen.github.io/microNeo/

### 提交方式
**直接发 PR**（不通过 Issue 评论）。

### 提交位置
主版面 README.md 最顶部 `### YYYY 年 M 月 D 号添加` 区块，新增：

```
#### sollawen - [Github](https://github.com/sollawen)
* :white_check_mark: [microNeo](https://github.com/sollawen/microNeo)：和 OpenCode、Claude 等 AI agent 一起讨论计划与方案的终端应用。选中文档中任意段落 -> 写下批注 -> 一键发送，让 AI 准确知道你想改哪段、怎么改，无需 ctrl-c/ctrl-v，让手指告别酸痛。 - [更多介绍](https://sollawen.github.io/microNeo/)
```

---

## 二、执行步骤（clone 到 /tmp/，标准 git 工作流）

⚠️ **注意：本仓库默认分支是 `master`，不是 `main`**（fork + upstream 都是）

### 1. 同步 fork（fork 已落后上游 44 天）
```bash
gh repo sync sollawen/chinese-independent-developer \
  --source 1c7/chinese-independent-developer \
  --branch master \
  --force
```

### 2. 克隆到 /tmp/
```bash
gh repo clone sollawen/chinese-independent-developer /tmp/cid-fork
cd /tmp/cid-fork
git remote add upstream https://github.com/1c7/chinese-independent-developer.git
git fetch upstream
git checkout -b add-microneo-master
```

### 3. 用 Python 在 README.md 顶部插入 microNeo 条目
```bash
python3 <<'PYEOF'
with open('README.md', 'r') as f:
    content = f.read()

insertion = (
    "#### sollawen - [Github](https://github.com/sollawen)\n"
    "* :white_check_mark: [microNeo](https://github.com/sollawen/microNeo)：和 OpenCode、Claude 等 AI agent 一起讨论计划与方案的终端应用。选中文档中任意段落 -> 写下批注 -> 一键发送，让 AI 准确知道你想改哪段、怎么改，无需 ctrl-c/ctrl-v，让手指告别酸痛。 - [更多介绍](https://sollawen.github.io/microNeo/)\n\n"
)

import re
match = re.search(r'^(### \d+ 年 \d+ 月 \d+ 号添加\s*\n)', content, re.MULTILINE)
if not match:
    raise SystemExit("未找到插入点，请手动检查 README.md")

new_content = content[:match.end()] + insertion + content[match.end():]
with open('README.md', 'w') as f:
    f.write(new_content)
print("OK")
PYEOF
```

### 4. 校验
```bash
git diff --check                        # 无空白错误
git diff README.md | head -20           # 人工看一眼修改
curl -I -o /dev/null -w "%{http_code}\n" https://github.com/sollawen/microNeo
curl -I -o /dev/null -w "%{http_code}\n" https://sollawen.github.io/microNeo/
```
两个 curl 应输出 `200`。

### 5. Commit + Push
```bash
git add README.md
git commit -m "新增：microNeo（vibe coding 终端 AI 协作应用）"
git push -u origin add-microneo-master
```

### 6. 写 PR body
```bash
cat > /tmp/cid-PR_BODY.md <<'EOF'
## 新增

在主版面列表中新增 microNeo：

- **产品名**：microNeo
- **定位**：和 OpenCode、Claude 等 AI agent 一起讨论计划与方案的终端应用
- **仓库**：https://github.com/sollawen/microNeo
- **官网**：https://sollawen.github.io/microNeo/

选中文档中任意段落 -> 写下批注 -> 一键发送，让 AI 准确知道你想改哪段、怎么改，无需 ctrl-c/ctrl-v，让手指告别酸痛。

## 验证

- [x] `git diff --check` 通过
- [x] 仓库链接可访问
- [x] 官网链接可访问
- [x] 状态图标 `:white_check_mark:`（已上线）
EOF
```

### 7. 提 PR（注意 `--base master`，不是 main）
```bash
gh pr create \
  --repo 1c7/chinese-independent-developer \
  --title "新增：microNeo（vibe coding 终端 AI 协作应用）" \
  --body-file /tmp/cid-PR_BODY.md \
  --base master \
  --head sollawen:add-microneo-master
```

### 8. 清理（merge 之后）
```bash
rm -rf /tmp/cid-fork
rm /tmp/cid-PR_BODY.md
gh repo delete sollawen/chinese-independent-developer --yes
```

---

## 三、风险预案

| 场景 | 应对 |
|------|------|
| 被告知"这是开发者工具" | 不争辩。同时改程序员版面描述（见下文）。 |
| 让改介绍语 | 配合，改完直接 amend commit。 |
| 24 小时没回 | 礼貌 ping 一次。 |

---

## 四、检查清单

- [ ] `gh repo sync` 同步 fork 到上游 master
- [ ] clone 到 `/tmp/cid-fork`
- [ ] 创建分支 `add-microneo-master`
- [ ] Python 脚本插入 microNeo 条目
- [ ] `git diff --check` 通过
- [ ] 两个链接 `curl -I` 验证 200
- [ ] commit + push
- [ ] `gh pr create --base master`
- [ ] 等 merge
- [ ] 清理：`rm -rf /tmp/cid-fork` + `gh repo delete fork`
