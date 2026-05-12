#!/bin/sh
# 容器启动时把环境变量注入到运行时配置。nginx:alpine 的官方 entrypoint
# 会 source 此目录下所有 *.sh，故无需自定义 ENTRYPOINT。
set -eu

TEMPLATE=/etc/aitask/runtime-config.template.js
TARGET=/usr/share/nginx/html/runtime-config.js

if [ ! -f "$TEMPLATE" ]; then
  echo "[runtime-config] template missing at $TEMPLATE" >&2
  exit 1
fi

# 仅替换显式枚举的变量，避免误触 ${unrelated} 字面量。
envsubst '${VITE_API_BASE_URL} ${VITE_WS_BASE_URL}' < "$TEMPLATE" > "$TARGET"

echo "[runtime-config] wrote $TARGET (api=${VITE_API_BASE_URL:-<empty>} ws=${VITE_WS_BASE_URL:-<empty>})"
