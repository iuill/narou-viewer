ARG PLAYWRIGHT_IMAGE_VERSION=1.58.2-node20-bookworm-slim-chromium-headless-shell-webkit-curl-amd64
ARG PLAYWRIGHT_BASE_IMAGE=ghcr.io/iuill/narou-viewer-playwright:${PLAYWRIGHT_IMAGE_VERSION}

FROM ${PLAYWRIGHT_BASE_IMAGE}

ARG PLAYWRIGHT_IMAGE_VERSION
ARG PLAYWRIGHT_BASE_IMAGE

LABEL io.narou-viewer.playwright-image-version="${PLAYWRIGHT_IMAGE_VERSION}"
LABEL io.narou-viewer.playwright-base-image="${PLAYWRIGHT_BASE_IMAGE}"

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    fonts-ipaexfont-gothic \
    fonts-ipaexfont-mincho \
    fonts-noto-cjk \
    fonts-noto-cjk-extra \
 && fc-cache -f \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*
