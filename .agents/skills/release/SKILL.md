---
name: release
description: microNeo release workflow. Use when user asks to release a version on github.
---

## Steps

```
1. 检查 CHANGELOG.md
   - 如果顶部是 [Unreleased]，改成 [X.Y.Z] - YYYY-MM-DD
   - 格式：## [X.Y.Z] - YYYY-MM-DD （方括号、无 v 前缀、ISO 日期）

2. Commit all
   git add -A
   git commit -m "Release X.Y.Z"

3. 打 tag
   git tag vX.Y.Z

4. Push
   git push origin master
   git push origin vX.Y.Z
   （tag push 触发 .github/workflows/release.yaml 自动发布）
```


## Reference

- Workflow: `.github/workflows/release.yaml` (extracts CHANGELOG section via `tools/extract-changelog.sh`, publishes with `softprops/action-gh-release@v2`)
