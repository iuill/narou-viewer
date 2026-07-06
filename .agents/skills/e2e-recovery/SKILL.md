---
name: e2e-recovery
description: Use when narou-viewer E2E fixtures, resident services, permissions, or state need initialization, restart, reset, or repair in local dev containers, Codespaces, or CI.
---

# E2E Recovery

この skill は `narou-viewer` の E2E fixture、常駐 service、権限、state を安全に立て直すための手順です。詳細方針は [`README.md`](../../../README.md) と [`AGENTS.md`](../../../AGENTS.md) を優先し、ここでは復旧系の判断だけを短く固定します。read-only smoke の実行判断は [`.agents/skills/e2e-smoke/SKILL.md`](../e2e-smoke/SKILL.md) を使います。

## 環境ごとの既定

### VS Code Dev Container / ローカル開発コンテナ

- Docker が使えるなら、既定は `bun run e2e:test:container`。
- 常駐サービスだけ先に立てたいときだけ `bun run e2e:services:up` を使う。
- `bun run e2e:test` も使えるが、Docker が使える環境では内部的に container runner 寄りになる。
- `DOCKER_HOST=unix:///run/devcontainer-docker-proxy/docker.sock` を向いているのに `docker version` が `no such file or directory` や `permission denied while trying to connect to the docker API` で落ちる場合は、まず `bash .devcontainer/scripts/ensure-docker-socket-proxy.sh` を再実行する。
- 再実行後は `docker version` または `bun run e2e:test:container` をそのまま再試行する。`scripts/e2e-compose.sh` 側にも proxy 再初期化と `/var/run/docker.sock` への fallback が入っている。
- `DOCKER_HOST=unix:///var/run/docker.sock` の直指定は切り分け用の一時手段としてはよいが、正規の復旧手順として常用しない。

### GitHub Codespaces

- `viewer-api-e2e` と `viewer-web-e2e` は起動時から常駐している前提で扱う。
- 既定は `bun run e2e:test:container` を先に試す。
- `bun run e2e:services:up` は不足サービスの補完には使えるが、毎回の最初の一手にはしない。
- `CODESPACES=true` のとき `e2e:services:up` は、必要な service が全部動いていれば `docker compose up` を skip する。
- `client version ... too new` / `Maximum supported API version is ...` が出たら、`DOCKER_API_VERSION` を明示して再実行する。

```bash
export DOCKER_API_VERSION=1.43
bun run e2e:test:container
```

### GitHub Actions

- CI では既定を `E2E_SERVICE_USER=0:0 bun run e2e:test:container` とする。
- 権限や所有者が怪しいときは、その前に `bun run e2e:repair` を挟む。
- artifact 量は `GITHUB_ACTIONS=true` 前提の screenshot / trace 設定に寄るので、手元の実行結果と差が出ても不思議ではない。

### Docker が使えないコンテナ環境

- `bun run e2e:test` は local runner 側へ寄るが、`PLAYWRIGHT_BASE_URL` が未設定だと失敗することがある。
- この環境では、`PLAYWRIGHT_BASE_URL` を明示して `bun run e2e:test:local` を使うか、Docker が使える場所へ戻って container runner を使う。

## 基本手順

### 初回または fixture 未作成

```bash
bun run e2e:fixture:init
bun run e2e:services:up
bun run e2e:test:container
```

### state だけ初期化したい

```bash
bun run e2e:state:reset
```

### fixture を明示的に作り直したい

```bash
bun run e2e:fixture:rebuild
```

### 権限や所有者が壊れた

```bash
bun run e2e:repair
```

### サービス設定が古そうで立て直したい

```bash
bun run e2e:services:down
bun run e2e:services:up
```

## 判断メモ

- `viewer-web-e2e` や `viewer-api-e2e` が古い設定を掴んでいそうなら、まず `e2e:services:down` と `e2e:services:up` を試す。
- Dev Container の Docker proxy 不調は、まず `bash .devcontainer/scripts/ensure-docker-socket-proxy.sh` の再実行で立て直す。
- `data_e2e` の作品 fixture は重いので、必要がない限り `e2e:fixture:rebuild` は使わない。
- 復旧後の確認は `bun run e2e:test:container` を既定にする。
- Dev Container / Codespaces では、`PLAYWRIGHT_BASE_URL` 未指定時の既定接続先は `http://viewer-web-e2e:15173` になる。
- GitHub Actions でローカル実行と同じ所有者前提にすると `data_e2e` や artifact の権限差で転びやすい。
