ARG PLAYWRIGHT_IMAGE_VERSION=v1.63.0-noble
ARG PLAYWRIGHT_BASE_IMAGE=mcr.microsoft.com/playwright:${PLAYWRIGHT_IMAGE_VERSION}

FROM ${PLAYWRIGHT_BASE_IMAGE}

COPY --from=oven/bun:1.4.4 /usr/local/bin/bun /usr/local/bin/bun

# Use a Node binary linked against older glibc, compatible with Ubuntu 24.04.
COPY --from=node:24.22.1-bookworm-slim /usr/local/bin/node /usr/local/bin/node

ARG PLAYWRIGHT_IMAGE_VERSION
ARG PLAYWRIGHT_BASE_IMAGE

LABEL io.narou-viewer.playwright-image-version="${PLAYWRIGHT_IMAGE_VERSION}"
LABEL io.narou-viewer.playwright-base-image="${PLAYWRIGHT_BASE_IMAGE}"

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get upgrade -y \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    fonts-ipaexfont-gothic \
    fonts-ipaexfont-mincho \
    fonts-noto-cjk \
    fonts-noto-cjk-extra \
 && fc-cache -f \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*
