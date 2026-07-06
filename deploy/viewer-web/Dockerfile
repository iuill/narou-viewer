FROM oven/bun:1.3.11 AS build

WORKDIR /workspace

COPY package.json bun.lock bunfig.toml .bun-version tsconfig.base.json ./
COPY scripts/run-with-root-env.mjs ./scripts/run-with-root-env.mjs
COPY apps/viewer-web/package.json apps/viewer-web/tsconfig.json ./apps/viewer-web/

RUN bun run install:locked

COPY apps/viewer-web ./apps/viewer-web

ARG VITE_APP_GIT_SHA=""
ARG VITE_APP_GIT_COMMIT_DATE=""
ARG VITE_PUBLIC_ASSET_BASE_URL=""

RUN if [ -z "${VITE_APP_GIT_SHA}" ] || [ -z "${VITE_APP_GIT_COMMIT_DATE}" ]; then \
    printf '%s\n' "VITE_APP_GIT_SHA and VITE_APP_GIT_COMMIT_DATE are required for production viewer-web builds." >&2; \
    exit 1; \
  fi; \
  VITE_APP_GIT_SHA="${VITE_APP_GIT_SHA}" \
  VITE_APP_GIT_COMMIT_DATE="${VITE_APP_GIT_COMMIT_DATE}" \
  VITE_PUBLIC_ASSET_BASE_URL="${VITE_PUBLIC_ASSET_BASE_URL}" \
  bun run build:web

FROM nginx:1.27-alpine

COPY --from=build /workspace/apps/viewer-web/dist /usr/share/nginx/html
COPY deploy/viewer-web/default.conf /etc/nginx/conf.d/default.conf
