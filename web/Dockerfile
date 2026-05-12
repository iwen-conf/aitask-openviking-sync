# syntax=docker/dockerfile:1.7
# 多阶段构建：node 22 (builder) → nginx 1.27 alpine (runtime)
# 目标：runtime image < 80MB，运行时由 envsubst 注入 base URL。

FROM node:22-alpine AS builder

ENV CI=true \
    PNPM_HOME=/pnpm \
    PATH=/pnpm:$PATH

WORKDIR /workspace

RUN corepack enable

# 锁文件优先复制以最大化层缓存。pnpm-workspace.yaml 携带 allowBuilds
# 白名单（pnpm v10+ 强制要求），缺它会让 frozen-lockfile install 失败。
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

# 复制业务源码后构建。`.dockerignore` 已排除 node_modules/dist。
COPY . .
RUN pnpm build

# ----------------------------- runtime ------------------------------------

FROM nginx:1.27-alpine AS runtime

# nginx:alpine 默认包含 envsubst（gettext-envsubst 二进制），无需额外安装。

# 自定义站点配置覆盖默认 default.conf（保留官方主 nginx.conf）。
COPY nginx.conf /etc/nginx/conf.d/default.conf

# 静态构建产物。
COPY --from=builder /workspace/dist /usr/share/nginx/html

# 运行时注入：模板 + 启动钩子。
COPY runtime-config.template.js /etc/aitask/runtime-config.template.js
COPY docker-entrypoint.d/40-runtime-config.sh /docker-entrypoint.d/40-runtime-config.sh
RUN chmod +x /docker-entrypoint.d/40-runtime-config.sh

EXPOSE 80

HEALTHCHECK --interval=10s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -q -O - http://127.0.0.1/healthz >/dev/null 2>&1 || exit 1

# 沿用 nginx:alpine 默认 ENTRYPOINT 与 CMD（会先执行 /docker-entrypoint.d/*.sh）。
