#!/bin/sh
# microNeo one-line installer
#
#   curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/tools/install.sh | sh
#
# 国内若 raw.githubusercontent.com 被限流(429)/不可达，可改用 jsDelivr 源：
#
#   curl -fsSL https://cdn.jsdelivr.net/gh/sollawen/microNeo@master/tools/install.sh | sh
#
# 本脚本内置多源容错：获取版本号与下载二进制都会按顺序尝试多个源，
# 优先 GitHub 官方源（海外最快），失败自动 fallback 到国内可达的镜像源，
# 单一源被限流或不可达不会阻断安装。

set -e

REPO="sollawen/microNeo"
INSTALL_DIR="$HOME/.local/bin"

# 连不上的源快速跳过，避免长时间卡住
CONNECT_TIMEOUT=10
MAX_TIMEOUT=180

# ------------------------------------------------------------------------------
# 多源容错工具函数
# ------------------------------------------------------------------------------

# 从 GitHub API 获取最新版本号（去掉前缀 v），失败输出空。
_fetch_version_github() {
    curl -fsSL --connect-timeout "$CONNECT_TIMEOUT" --max-time 30 \
        "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
        | grep '"tag_name"' \
        | sed -E 's/.*"v?([^"]+)".*/\1/' \
        | head -1
}

# 从 jsDelivr data API 获取最新版本号（已按版本倒序，取第一个）。
_fetch_version_jsdelivr() {
    curl -fsSL --connect-timeout "$CONNECT_TIMEOUT" --max-time 30 \
        "https://data.jsdelivr.com/v1/packages/gh/${REPO}" 2>/dev/null \
        | sed -nE 's/.*"version"[[:space:]]*:[[:space:]]*"v?([^"]+)".*/\1/p' \
        | head -1
}

# 输出最新版本号到 stdout；提示信息走 stderr，不污染 stdout。
# 用法：VERSION=$(get_latest_version)
get_latest_version() {
    _src=""
    _v=$(_fetch_version_github) || _v=""
    if [ -z "$_v" ]; then
        _src="(via jsDelivr)"
        _v=$(_fetch_version_jsdelivr) || _v=""
    else
        _src="(via GitHub API)"
    fi
    [ -n "$_v" ] || return 1
    printf '  最新版本 %s %s\n' "$_v" "$_src" >&2
    printf '%s\n' "$_v"
}

# 依次尝试多个 URL，下载到第一个参数指定的文件；全部失败返回非零。
# 用法：download_to <output_file> <url1> [url2] ...
download_to() {
    _out="$1"; shift
    for _url in "$@"; do
        _host=$(printf '%s' "$_url" | sed -E 's#https?://([^/]+).*#\1#')
        printf '  尝试下载源 %s ... ' "$_host" >&2
        if curl -fSL --connect-timeout "$CONNECT_TIMEOUT" --max-time "$MAX_TIMEOUT" \
                "$_url" -o "$_out" 2>/dev/null; then
            printf 'ok\n' >&2
            return 0
        fi
        printf '失败，换源\n' >&2
    done
    return 1
}

# ------------------------------------------------------------------------------
# 环境检测
# ------------------------------------------------------------------------------

case "$(uname -s)" in
    Linux*)     OS="linux" ;;
    Darwin*)    OS="macos" ;;
    CYGWIN*|MINGW*|MSYS*) OS="windows" ;;
    *)          echo "Unsupported OS: $(uname -s)" && exit 1 ;;
esac

case "$(uname -m)" in
    x86_64)     ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    i386|i686)  ARCH="386" ;;
    *)          echo "Unsupported architecture: $(uname -m)" && exit 1 ;;
esac

# ------------------------------------------------------------------------------
# 获取版本号（多源）
# ------------------------------------------------------------------------------

VERSION=$(get_latest_version) || {
    echo "Failed to fetch latest version from all sources."
    echo "请检查网络，或手动指定版本下载："
    echo "  https://github.com/${REPO}/releases"
    exit 1
}

# ------------------------------------------------------------------------------
# 构造文件名
# ------------------------------------------------------------------------------

case "${OS}" in
    macos)
        EXT="tar.gz"
        if [ "$ARCH" = "arm64" ]; then
            FILENAME="microneo-${VERSION}-macos-arm64.tar.gz"
        else
            FILENAME="microneo-${VERSION}-osx.tar.gz"
        fi
        ;;
    linux)
        EXT="tar.gz"
        if [ "$ARCH" = "arm64" ]; then
            FILENAME="microneo-${VERSION}-linux-arm64.tar.gz"
        else
            FILENAME="microneo-${VERSION}-linux64.tar.gz"
        fi
        ;;
    windows)
        EXT="zip"
        if [ "$ARCH" = "arm64" ]; then
            FILENAME="microneo-${VERSION}-win-arm64.zip"
        else
            FILENAME="microneo-${VERSION}-win64.zip"
        fi
        ;;
esac

# 官方下载地址（GitHub Release，会 302 到 objects.githubusercontent.com）
GITHUB_URL="https://github.com/${REPO}/releases/download/v${VERSION}/${FILENAME}"

# ------------------------------------------------------------------------------
# 下载（多源 fallback）
# ------------------------------------------------------------------------------

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT INT TERM
cd "$TEMP_DIR"

echo "Downloading microNeo v${VERSION} for ${OS}-${ARCH}..."
if ! download_to "microneo.${EXT}" \
        "$GITHUB_URL" \
        "https://ghproxy.net/${GITHUB_URL}" \
        "https://ghfast.top/${GITHUB_URL}" \
        "https://ghproxy.com/${GITHUB_URL}" \
        "https://mirror.ghproxy.com/${GITHUB_URL}"; then
    echo "所有下载源均失败。请检查网络，或手动下载："
    echo "  $GITHUB_URL"
    exit 1
fi

# ------------------------------------------------------------------------------
# 解压 & 安装
# ------------------------------------------------------------------------------

echo "Installing..."
case "${EXT}" in
    tar.gz)  tar -xzf "microneo.${EXT}" ;;
    zip)     unzip -q "microneo.${EXT}" ;;
esac

# 压缩包内结构为 microneo-VERSION/ 子目录，需找出二进制
BINARY=$(find . -name microneo -type f | head -1)
if [ -z "$BINARY" ]; then
    echo "Error: microneo binary not found in archive"
    exit 1
fi

mkdir -p "$INSTALL_DIR"
mv "$BINARY" "$INSTALL_DIR/microneo"
chmod +x "$INSTALL_DIR/microneo"

echo "microNeo v${VERSION} installed to ${INSTALL_DIR}/microneo"

# 检查安装目录是否在 PATH 中
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) printf '\nAdd %s to your PATH:\n' "$INSTALL_DIR"
       printf "  echo 'export PATH=\"%s:\$PATH\"' >> ~/.bashrc && source ~/.bashrc\n" "$INSTALL_DIR" ;;
esac

echo "Run 'microneo --version' to verify"
