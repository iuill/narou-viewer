# デプロイと self-host

このドキュメントは、ローカルまたは任意の self-host 環境で使える compose サンプルと配置時の注意点をまとめる。

## 方針

- この repository は app source、Dockerfile、汎用 self-host compose sample を管理する。
- `docker-compose.prod.yml` は HTTP の reverse proxy sample であり、既定では host の `127.0.0.1` に bind する。TLS と認証は前段の reverse proxy、VPN、tunnel、hosting platform などで扱う。
- `data/` や named volume に入る本文、raw HTML、画像、state、AI model output は runtime data として扱い、repository へ保存しない。
- API key や LLM provider の認証情報は `.env.local`、shell environment、hosting platform secrets などで渡し、Git 管理しない。

## self-host compose

repository root で次を実行する。

```bash
docker compose -f docker-compose.prod.yml up -d --build
```

既定では `NAROU_VIEWER_HTTP_PORT` が未指定なら同一 host の `127.0.0.1:8080` で開ける。

```bash
NAROU_VIEWER_HTTP_PORT=18080 docker compose -f docker-compose.prod.yml up -d --build
```

外部 interface へ直接 bind する場合は、TLS と認証を別途設定したうえで bind address を明示する。

```bash
NAROU_VIEWER_HTTP_BIND=0.0.0.0 docker compose -f docker-compose.prod.yml up -d --build
```

構成:

- `reverse-proxy`: Nginx。HTTP で `viewer-web` と `/api/*` を同一 origin にまとめる。
- `viewer-web`: `deploy/viewer-web/Dockerfile` で build した静的ファイル配信。
- `viewer-api`: `deploy/viewer-api-go/Dockerfile` で build した API service。
- `novel-fetcher`: 取得 sidecar。`viewer-api` から内部 network 経由で使う。
- `shared-data`: `novel-fetcher` の保存データと `state/` を置く named volume。

## env

任意の外部連携を使う場合は、compose の `.env` または shell environment で値を渡す。

```bash
AI_GENERATION_SETTINGS_MASTER_PASSPHRASE=...
OPENROUTER_API_BASE_URL=...
GOOGLE_BOOKS_API_KEY=...
```

`AI_GENERATION_SETTINGS_MASTER_PASSPHRASE` は保存済み AI generation settings の暗号化・復号に使う。
設定する場合は固定値として保管し、紛失しないようにする。

`novel-fetcher` の既定 User-Agent は、実サイト互換性を優先した browser-like な値にしている。ツール名や repository URL を識別できる UA は既定では送らない。これは self-host 利用者ごとの運用まで project に紐づいて見えることを避けるための方針である。

利用者が自身の運用方針に合わせて UA を明示したい場合は、compose の `.env` または shell environment で `NOVEL_FETCHER_USER_AGENT` を設定する。

## 公開時の注意

- `docker-compose.prod.yml` 自体は TLS や認証を終端せず、既定では `127.0.0.1` に bind する。
- インターネットへ公開する場合は、Caddy、Traefik、Nginx、Cloudflare Tunnel、VPN など任意の前段で TLS と認証を設定する。
- `novel-fetcher` の `33006` は publish しない。取得 sidecar 操作は `viewer-api` の `/api/fetcher/*` 経由に限定する。
- runtime data は file / SQLite を個別に copy せず、[`state-backup`](state-backup.md) で viewer-api / novel-fetcher を停止した 1 generation の暗号化 archive として取得・復元する。

## 確認

起動後、同一 host から次を確認する。

```bash
curl -fsS http://localhost:${NAROU_VIEWER_HTTP_PORT:-8080}/api/health
curl -fsS http://localhost:${NAROU_VIEWER_HTTP_PORT:-8080}/api/system/status
```

外部 URL に向けた read-only smoke を行う場合は、`PLAYWRIGHT_BASE_URL` を指定して smoke spec を実行する。

```bash
PLAYWRIGHT_BASE_URL=https://example.invalid bun run e2e:test:smoke
```
