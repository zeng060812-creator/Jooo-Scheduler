#!/usr/bin/env sh
# 若在 Linux/macOS/Termux 使用，请保证 PATH 中已有 Go。
set -eu
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT/source/jood"
mkdir -p "$ROOT/bin"
CGO_ENABLED=0 GOOS=android GOARCH=arm64 \
  go build -trimpath -ldflags='-s -w' -o "$ROOT/bin/jood" .
echo "已生成: $ROOT/bin/jood"
