#!/usr/bin/env bash
# 源码构建镜像：./build.sh [--up] [--no-cache]
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

UP=0
BUILD_ARGS=()
for arg in "$@"; do
    case "$arg" in
        --up) UP=1 ;;
        --no-cache) BUILD_ARGS+=(--no-cache) ;;
        *) echo "未知参数: $arg" >&2; exit 1 ;;
    esac
done

if [ ! -f .env ]; then
    echo "缺少 .env，请先执行: cp .env.example .env 并填写配置" >&2
    exit 1
fi

# 版本号：最近tag-短commit，有未提交改动时追加 -dirty（与 CI 规则一致）
TAG="$(git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0)"
VERSION="${TAG}-$(git rev-parse --short=7 HEAD)"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
    VERSION="${VERSION}-dirty"
fi
export VERSION

mkdir -p data/log

echo "构建版本: ${VERSION}"
# ${arr[@]+...} 写法兼容 bash < 4.4 在 set -u 下展开空数组
docker compose build ${BUILD_ARGS[@]+"${BUILD_ARGS[@]}"}

if [ "$UP" -eq 1 ]; then
    NETWORK="${EXTERNAL_NETWORK:-$(grep -E '^EXTERNAL_NETWORK=' .env | cut -d= -f2- | tr -d "\"' ")}"
    if ! docker network inspect "$NETWORK" >/dev/null 2>&1; then
        echo "docker 网络不存在: ${NETWORK}，请检查 .env 中的 EXTERNAL_NETWORK" >&2
        exit 1
    fi
    docker compose up -d
    docker compose ps
fi
