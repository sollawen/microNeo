---
name: release
description: microNeo release workflow. Use when user asks to release a version on github.
---

## Steps

```bash
# 1. Lightweight tag (no -a, no -m). Format: vX.Y.Z
git tag vX.Y.Z

# 2. Push — tag push triggers .github/workflows/release.yaml
git push origin master
git push origin vX.Y.Z
```


## Reference

- Workflow: `.github/workflows/release.yaml` (extracts CHANGELOG section via `tools/extract-changelog.sh`, publishes with `softprops/action-gh-release@v2`)
