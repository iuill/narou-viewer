FROM mcr.microsoft.com/devcontainers/typescript-node:1-22-bookworm

ENV TZ=Asia/Tokyo
ARG GO_VERSION=1.25.12

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bubblewrap \
    fonts-ipaexfont-gothic \
    fonts-ipaexfont-mincho \
    fonts-noto-cjk \
    fonts-noto-cjk-extra \
    locales \
    python-is-python3 \
    ripgrep \
    sqlite3 \
    time \
    tzdata \
 && sed -i 's/^# *ja_JP.UTF-8 UTF-8/ja_JP.UTF-8 UTF-8/' /etc/locale.gen \
 && locale-gen ja_JP.UTF-8 \
 && update-locale LANG=ja_JP.UTF-8 LC_ALL=ja_JP.UTF-8 \
 && fc-cache -f \
 && ln -snf "/usr/share/zoneinfo/$TZ" /etc/localtime \
 && echo "$TZ" > /etc/timezone \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

ENV LANG=ja_JP.UTF-8 \
    LC_ALL=ja_JP.UTF-8 \
    GOTOOLCHAIN=local

RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "${arch}" in \
      amd64) go_arch="amd64" ;; \
      arm64) go_arch="arm64" ;; \
      *) echo "unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac; \
    rm -rf /usr/local/go; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${go_arch}.tar.gz" \
      | tar -C /usr/local -xz; \
    ln -sf /usr/local/go/bin/go /usr/local/bin/go; \
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt; \
    /usr/local/go/bin/go version | grep -F "go${GO_VERSION} "

ENV PATH=/usr/local/go/bin:${PATH}

# コンテナ再作成のたびに install-bun-and-deps.sh がツール群を再ダウンロードしないよう
# イメージに焼き込む。バージョンは install-bun-and-deps.sh のデフォルトと一致させること
# (不一致の場合は postCreate 側が検出して入れ直すため壊れはしないが、焼き込みが無駄になる)。
ARG OPENAI_CODEX_VERSION=0.139.0
ARG GITHUB_COPILOT_VERSION=1.0.61
ARG PLAYWRIGHT_CLI_VERSION=0.1.9
ARG SERENA_AGENT_VERSION=1.3.0
ARG SCC_VERSION=v3.7.0
ARG SCC_LINUX_X86_64_SHA256=3d9d65b00ca874c2b29151abe7e1480736f5229edc3ce8e4b2791460cdfabf5a
ARG BETTERLEAKS_VERSION=1.6.1
ARG BETTERLEAKS_LINUX_X64_SHA256=fbefc700a0bd4522cc952dd2a8f259cdb80526d7e60114aca19bb2d6fdc80f81
ARG BETTERLEAKS_LINUX_ARM64_SHA256=bab9688ba968264ace67b608fc7a7d8f5e61218cde70029d32cbc894e3808fdf

RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    if [ "${arch}" = "amd64" ]; then \
      tmpdir="$(mktemp -d)"; \
      curl -fsSL "https://github.com/boyter/scc/releases/download/${SCC_VERSION}/scc_Linux_x86_64.tar.gz" -o "${tmpdir}/scc_Linux_x86_64.tar.gz"; \
      echo "${SCC_LINUX_X86_64_SHA256}  ${tmpdir}/scc_Linux_x86_64.tar.gz" | sha256sum -c -; \
      tar -xzf "${tmpdir}/scc_Linux_x86_64.tar.gz" -C "${tmpdir}"; \
      install -m 0755 "${tmpdir}/scc" /usr/local/bin/scc; \
      rm -rf "${tmpdir}"; \
    else \
      echo "scc is not preinstalled for architecture ${arch}; postCreate can install it when a pinned archive is configured."; \
    fi

RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "${arch}" in \
      amd64) betterleaks_arch="x64"; checksum="${BETTERLEAKS_LINUX_X64_SHA256}" ;; \
      arm64) betterleaks_arch="arm64"; checksum="${BETTERLEAKS_LINUX_ARM64_SHA256}" ;; \
      *) echo "unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac; \
    archive="betterleaks_${BETTERLEAKS_VERSION}_linux_${betterleaks_arch}.tar.gz"; \
    tmpdir="$(mktemp -d)"; \
    curl -fsSL "https://github.com/betterleaks/betterleaks/releases/download/v${BETTERLEAKS_VERSION}/${archive}" -o "${tmpdir}/${archive}"; \
    echo "${checksum}  ${tmpdir}/${archive}" | sha256sum -c -; \
    tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}" betterleaks; \
    install -m 0755 "${tmpdir}/betterleaks" /usr/local/bin/betterleaks; \
    rm -rf "${tmpdir}"

RUN su node -c "umask 0002 && npm install -g '@playwright/cli@${PLAYWRIGHT_CLI_VERSION}'"

# remoteUser (node) のホームに置くことで postCreate のインストール済み判定を通す
USER node

RUN curl -fsSL https://bun.sh/install | bash \
 && /home/node/.bun/bin/bun add -g "@openai/codex@${OPENAI_CODEX_VERSION}" "@github/copilot@${GITHUB_COPILOT_VERSION}" \
 && mkdir -p /home/node/.bun/install/cache

RUN curl -LsSf https://astral.sh/uv/install.sh | sh \
 && /home/node/.local/bin/uv tool install -p 3.13 "serena-agent==${SERENA_AGENT_VERSION}"

RUN sudo apt-get update \
 && playwright-cli install-browser chromium --with-deps \
 && playwright-cli install-browser webkit --with-deps \
 && sudo apt-get clean \
 && sudo rm -rf /var/lib/apt/lists/*

USER root
