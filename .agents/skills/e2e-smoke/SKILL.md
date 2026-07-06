---
name: e2e-smoke
description: Use when running narou-viewer read-only smoke checks against local, Dev Container, Codespaces, or generic self-host origins.
---

# E2E Smoke

この skill は `narou-viewer` の read-only smoke check をどの経路で実行するか判断するための手順です。fixture や常駐 service の復旧は [`.agents/skills/e2e-recovery/SKILL.md`](../e2e-recovery/SKILL.md) を使います。

## 何を使うか

### 通常のアプリ E2E

- 画面操作や複数 service 連携の確認は `bun run e2e:test:container` を基本にする。
- Docker が使える Dev Container / Codespaces / ローカルではこの経路が第一候補。

### read-only smoke spec だけを回したい

```bash
bun run e2e:test:smoke
```

- `e2e/prod-readonly-smoke.spec.ts` だけを実行する。
- `state` 初期化は行わない。
- `PLAYWRIGHT_BASE_URL` を指定すれば、generic self-host origin の軽い確認にも使える。

### `playwright-e2e` コンテナを使うローカル向け shortcut

```bash
bun run e2e:test:smoke:container
```

- これは `playwright-e2e` を使うローカル向け shortcut。
- 既定では内部 E2E service を対象にする。

## 環境ごとの既定

### VS Code Dev Container / ローカル開発コンテナ

- Docker が使えるなら、通常 E2E は `bun run e2e:test:container`。
- `bun run e2e:test:smoke` は、`PLAYWRIGHT_BASE_URL` 未指定なら `http://viewer-web-e2e:15173` を向く。
- generic self-host origin を見るときは `PLAYWRIGHT_BASE_URL` を明示する。
- Docker socket proxy が不調なら、smoke 実行前に [`.agents/skills/e2e-recovery/SKILL.md`](../e2e-recovery/SKILL.md) の復旧手順へ戻る。

### GitHub Codespaces

- `CODESPACES=true` 前提で、`bun run e2e:test:container` を先に試す。
- `bun run e2e:test:smoke` も既定では `http://viewer-web-e2e:15173` を向く。
- Codespaces では常駐 E2E service 再利用が前提なので、外部 origin を見ない限り `PLAYWRIGHT_BASE_URL` は省略してよい。
- `client version ... too new` / `Maximum supported API version is ...` が出たら、`DOCKER_API_VERSION` を明示して再実行する。

### GitHub Actions

- 通常の CI では `E2E_SERVICE_USER=0:0 bun run e2e:test:container -- --project=...` の系統を使う。
- read-only smoke spec だけを局所的に見る場合は `bun run e2e:test:smoke` を使う。

### Docker が使えないコンテナ環境

- `bun run e2e:test:local` または `bun run e2e:test:smoke` を使う。
- `PLAYWRIGHT_BASE_URL` 未指定では既定接続先が不適切になることがあるので、target origin を明示する。

## generic self-host origin の read-only smoke

```bash
PLAYWRIGHT_BASE_URL=http://127.0.0.1:8080 bun run e2e:test:smoke
```

- `docker-compose.prod.yml` などで起動した汎用 self-host origin に対し、トップ画面、library API、read-only episode API を軽く確認する。
- state 初期化や fixture 再作成は行わない。
- origin 側のデータが空の場合、readable episode が見つからず失敗することがある。その場合はアプリの問題かデータ不足かを切り分けて報告する。

## 判断メモ

- generic self-host origin を見る場合、正規経路は `e2e:test:smoke` であり、`e2e:test:smoke:container` ではない。
- Dev Container / Codespaces では未指定の既定接続先が内部 service 名になるので、別 origin を見たいときは `PLAYWRIGHT_BASE_URL` を明示する。
- E2E service 自体が不安定なら smoke より先に [`.agents/skills/e2e-recovery/SKILL.md`](../e2e-recovery/SKILL.md) で立て直す。
