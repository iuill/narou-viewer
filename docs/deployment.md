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

## backup と restore

backup は `viewer-api` と `novel-fetcher` を停止し、共有 data root 全体を一度に copy する。
稼働中の file や SQLite、作品単位の部分 copy は正式な復旧手段として扱わない。

```bash
docker compose -f docker-compose.prod.yml stop viewer-api novel-fetcher
tar czf backup-$(date +%Y%m%d).tar.gz -C /path/to/data-root .
docker compose -f docker-compose.prod.yml start novel-fetcher viewer-api
```

保存先が暗号化されていない場合は、`age` などで archive を暗号化する。
暗号化、保存先、世代管理、retention は運用者または外部 backup 基盤が管理する。

```bash
age -r 'age1...' -o backup.tar.gz.age backup.tar.gz
```

restore は両 service を停止したまま、空の data root または新しい volume へ backup 全体を展開する。
既存 data への上書き restore と application binary だけを戻す downgrade はサポートしない。
rollback では upgrade 前の data root 全体と、それに対応する旧 build を組み合わせる。

```bash
tar xzf backup.tar.gz -C /path/to/empty-data-root
```

旧 `state-backup` が作成した `.tar.gz.age` は標準の age、gzip、tar 形式なので、専用 CLI がなくても復元できる。
archive 内の `manifest.json` は新 build では使用しない。

```bash
age -d -i /path/to/identity.txt backup.tar.gz.age | tar xz -C /path/to/empty-data-root
```

### 専用 backup tooling を削除する build への upgrade

旧 restore journal が残った状態で新 build を起動すると、未完了 restore の recovery を実行できない。
専用 backup tooling を含む build から初めて upgrade するときは、次の順序を守る。

1. 現行 build のまま `viewer-api` と `novel-fetcher` を停止する。
2. 現行 build の `state-backup recover` を実行し、中断 restore の rollback または cleanup を完了する。
3. data root 直下に `.state-restore-transaction.json`、`.restore-staging-*`、`.restore-rollback-*` が残っていないことを確認する。残っている場合は手動削除せず、現行 build の recovery で解消する。
4. 現行 build で最後の cold backup を取得する。
5. 新 build へ upgrade する。
6. 問題があれば、upgrade 前 backup と対応する旧 build の組で rollback する。

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
