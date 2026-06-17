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

# substr + == 做字符串匹配，避开 BSD/GNU awk 在 \[ \] 转义上的差异
# length("## [v]") = 5 + length(v)
awk -v v="${VERSION}" '
  substr($0, 1, 5 + length(v)) == "## [" v "]" { found=1; next }
  /^## \[[0-9]+\.[0-9]+\.[0-9]+/ { found=0 }
  found { print }
' CHANGELOG.md > "$OUT"

if [ ! -s "$OUT" ]; then
  echo "::error::No CHANGELOG.md entry found for v${VERSION}" >&2
  echo "::error::Available versions:" >&2
  grep -oE '## \[[0-9]+\.[0-9]+\.[0-9]+\]' CHANGELOG.md >&2 || true
  exit 1
fi
