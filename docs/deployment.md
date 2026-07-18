# デプロイと self-host

このドキュメントは、ローカルまたは任意の self-host 環境で使える compose サンプルと配置時の注意点をまとめる。

## 方針

- この repository は app source、Dockerfile、汎用 self-host compose sample を管理する。
- `docker-compose.prod.yml` は HTTP の self-host sample であり、`viewer-web` の Nginx を既定で host の `127.0.0.1` に bind する。TLS と認証は前段の reverse proxy、VPN、tunnel、hosting platform などで扱う。
- `data/` や named volume に入る本文、raw HTML、画像、state、AI model output は runtime data として扱い、repository へ保存しない。
- API key や LLM provider の認証情報は `.env.local`、shell environment、hosting platform secrets などで渡し、Git 管理しない。

## self-host compose

repository root で次を実行する。

```bash
docker compose -p narou-viewer -f docker-compose.prod.yml up -d --build
```

既定では `NAROU_VIEWER_HTTP_PORT` が未指定なら同一 host の `127.0.0.1:8080` で開ける。

```bash
NAROU_VIEWER_HTTP_PORT=18080 \
  docker compose -p narou-viewer -f docker-compose.prod.yml up -d --build
```

外部 interface へ直接 bind する場合は、TLS と認証を別途設定したうえで bind address を明示する。

```bash
NAROU_VIEWER_HTTP_BIND=0.0.0.0 \
  docker compose -p narou-viewer -f docker-compose.prod.yml up -d --build
```

`narou-viewer` は Compose project 名である。
初回起動後に別の名前へ変えると別の named volume が作られるため、`.env` の `COMPOSE_PROJECT_NAME` または運用記録へ保存し、停止、起動、backup、restore、診断で同じ値を使う。
checkout directory 名から暗黙に project 名を決めない。

構成:

- `viewer-web`: Nginx。`deploy/viewer-web/Dockerfile` で build した静的ファイルを配信し、`/api/*` を `viewer-api` へ転送して同一 origin にまとめる。
- `viewer-api`: `deploy/viewer-api-go/Dockerfile` で build した API service。
- `novel-fetcher`: 取得 sidecar。`viewer-api` から内部 network 経由で使う。
- `shared-data-init`: 初回起動時に共有 data の directory と owner を初期化する one-shot service。
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

標準 compose は共有 data root を `shared-data` named volume として管理する。
backup は `viewer-api` と `novel-fetcher` を停止し、この volume 全体を一度に保存する。
稼働中の file や SQLite、作品単位の部分 copy は正式な復旧手段として扱わない。

### named volume の backup

backup directory には checkout 外の絶対 path を指定し、Git 管理下へ archive を作成しない。
次の例は、停止中の `shared-data` から gzip archive を host へ stream し、展開可能性を確認してから最終名へ移動する。

```bash
set -euo pipefail

active_project=narou-viewer
backup_dir=/absolute/path/outside/checkout/narou-viewer-backups
backup_name="backup-narou-viewer-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
backup_archive="${backup_dir}/${backup_name}"
backup_partial="${backup_archive}.partial"
compose=(docker compose -p "$active_project" -f docker-compose.prod.yml)

umask 077
install -d -m 700 "$backup_dir"
test ! -e "$backup_archive"
test ! -e "$backup_partial"
test -n "$("${compose[@]}" ps -a -q viewer-api)"
test -n "$("${compose[@]}" ps -a -q novel-fetcher)"

"${compose[@]}" stop viewer-api novel-fetcher

"${compose[@]}" run --rm --no-deps -T \
  --entrypoint sh \
  shared-data-init \
  -ceu '
    test -f /data/.shared-data-init-ok
    test -d /data/state
    test -d /data/novel-fetcher
    tar czf - -C /data .
  ' >"$backup_partial"

test -s "$backup_partial"
tar tzf "$backup_partial" >/dev/null
mv "$backup_partial" "$backup_archive"
(
  cd "$backup_dir"
  sha256sum "$backup_name" >"${backup_name}.sha256"
)

"${compose[@]}" start novel-fetcher viewer-api
```

`active_project` には現在実際に稼働している project 名を指定する。
restore project を稼働系として採用した後は、その project 名へ置き換える。
`ps -a -q` と data marker の確認に失敗した場合は、空または過去の project を誤って選んでいる可能性があるため、backup を中止する。

backup command または検証が失敗した場合は service を停止したままにし、成功済み archive として扱わない。
`.partial` を削除して再試行する場合は、対象 path が `$backup_partial` と一致することを確認する。

archive には取得済み本文、画像、読書履歴、AI 利用履歴、暗号化済み credential、legacy の平文 credential が含まれ得る。
archive と checksum を repository へ追加せず、暗号化、保存先、世代管理、retention は運用者または外部 backup 基盤が管理する。

平文 archive を残さない場合は、service 停止中の `tar` stream を直接 `age` へ渡す。
実行環境には `age` CLI を用意し、recipient と復号 identity を backup 本体とは別に保管する。
復号後の `tar` stream を検証してから成功扱いにする。

```bash
set -euo pipefail

active_project=narou-viewer
backup_dir=/absolute/path/outside/checkout/narou-viewer-backups
encrypted_name="backup-narou-viewer-$(date -u +%Y%m%dT%H%M%SZ).tar.gz.age"
encrypted_archive="${backup_dir}/${encrypted_name}"
encrypted_partial="${encrypted_archive}.partial"
compose=(docker compose -p "$active_project" -f docker-compose.prod.yml)

umask 077
install -d -m 700 "$backup_dir"
test ! -e "$encrypted_archive"
test ! -e "$encrypted_partial"
test -n "$("${compose[@]}" ps -a -q viewer-api)"
test -n "$("${compose[@]}" ps -a -q novel-fetcher)"

"${compose[@]}" stop viewer-api novel-fetcher

"${compose[@]}" run --rm --no-deps -T \
  --entrypoint sh \
  shared-data-init \
  -ceu '
    test -f /data/.shared-data-init-ok
    test -d /data/state
    test -d /data/novel-fetcher
    tar czf - -C /data .
  ' | \
  age -r "$AGE_BACKUP_RECIPIENT" -o "$encrypted_partial"

test -s "$encrypted_partial"
age -d -i "$AGE_BACKUP_IDENTITY_FILE" "$encrypted_partial" | tar tzf - >/dev/null
mv "$encrypted_partial" "$encrypted_archive"
(
  cd "$backup_dir"
  sha256sum "$encrypted_name" >"${encrypted_name}.sha256"
)

"${compose[@]}" start novel-fetcher viewer-api
```

### 新しい named volume への restore

restore は既存 project を停止したまま、別 project 名に対応する新しい named volume へ backup 全体を展開する。
既存 volume への上書き restore と、application binary だけを戻す downgrade はサポートしない。

最初に archive の checksum と展開可能性を確認する。
次に restore 先の volume を作成し、空であることを展開直前にも検査する。

```bash
set -euo pipefail

backup_archive=/absolute/path/outside/checkout/narou-viewer-backups/backup-narou-viewer-YYYYMMDDTHHMMSSZ.tar.gz
source_project=narou-viewer
restore_project="narou-viewer-restore-$(date -u +%Y%m%dt%H%M%Sz)"
restore_volume="${restore_project}_shared-data"
source_compose=(docker compose -p "$source_project" -f docker-compose.prod.yml)
restore_compose=(docker compose -p "$restore_project" -f docker-compose.prod.yml)

test "$source_project" != "$restore_project"
test -n "$("${source_compose[@]}" ps -a -q viewer-api)"
test -n "$("${source_compose[@]}" ps -a -q novel-fetcher)"

(
  cd "$(dirname "$backup_archive")"
  sha256sum -c "$(basename "$backup_archive").sha256"
)
tar tzf "$backup_archive" >/dev/null

"${source_compose[@]}" stop

docker volume create \
  --label "com.docker.compose.project=${restore_project}" \
  --label "com.docker.compose.volume=shared-data" \
  "$restore_volume"

docker run --rm -i \
  -v "${restore_volume}:/data" \
  alpine:3.22 \
  sh -ceu '
    test -z "$(find /data -mindepth 1 -print -quit)"
    tar xzf - -C /data
    chown -R 1000:1000 /data
  ' <"$backup_archive"

docker run --rm \
  -v "${restore_volume}:/data:ro" \
  alpine:3.22 \
  sh -ceu '
    test -d /data/state
    test -d /data/novel-fetcher
    test -z "$(find /data -xdev \( ! -user 1000 -o ! -group 1000 \) -print -quit)"
    test -z "$(find /data -xdev -type l -print -quit)"
    test -z "$(find /data -xdev -perm /022 -print -quit)"
  '

"${restore_compose[@]}" up -d
test -z "$("${source_compose[@]}" ps --status running -q viewer-api novel-fetcher)"
```

`source_project` には restore 前に稼働している project 名を指定する。
復元後の確認が完了し、`restore_project` を正式な稼働系として採用したら、その値を `.env` の `COMPOSE_PROJECT_NAME` または同等の運用設定へ保存する。
以後の backup、診断、停止、起動では、保存した project 名を `active_project` として使う。
元の `source_project` は rollback 判断が終わるまで保持してよいが、新しい稼働系と同時に writer を起動しない。

暗号化 archive を restore する場合も checksum を先に検証する。
上の手順で新 volume を作成した後、平文 archive の展開 command だけを次の pipeline へ置き換え、後続の owner、mode、directory 検査を同様に実行する。
`age -d` の出力は展開用 container の標準入力へ渡す。
復号済み archive を checkout や一時 directory へ保存しない。

```bash
set -euo pipefail

encrypted_archive=/absolute/path/outside/checkout/narou-viewer-backups/backup-narou-viewer-YYYYMMDDTHHMMSSZ.tar.gz.age
restore_project=narou-viewer-restore-yyyymmddthhmmssz
restore_volume="${restore_project}_shared-data"

(
  cd "$(dirname "$encrypted_archive")"
  sha256sum -c "$(basename "$encrypted_archive").sha256"
)

age -d -i "$AGE_BACKUP_IDENTITY_FILE" "$encrypted_archive" | \
  docker run --rm -i \
    -v "${restore_volume}:/data" \
    alpine:3.22 \
    sh -ceu '
      test -z "$(find /data -mindepth 1 -print -quit)"
      tar xzf - -C /data
      chown -R 1000:1000 /data
    '
```

起動後は health、library、既読位置、栞、取得 task の状態を確認する。
確認に失敗した場合は restore project を停止し、元の volume を変更せずに保持する。
失敗した新 volume を削除する場合は、`$restore_volume` が意図した新 volume と一致し、元の volume ではないことを `docker volume inspect` で確認する。

```bash
restore_project=narou-viewer-restore-yyyymmddthhmmssz
restore_volume="${restore_project}_shared-data"
restore_compose=(docker compose -p "$restore_project" -f docker-compose.prod.yml)

"${restore_compose[@]}" down

docker volume inspect "$restore_volume"
```

inspect 結果が意図した新 volume と一致することを確認した後、その volume だけを削除する。

```bash
docker volume rm "$restore_volume"
```

rollback では upgrade 前の data root 全体 backup と、それに対応する旧 build を組み合わせる。

## state の診断

YAML、JSON、SQLite の version や形式に対応できない場合、各 service は起動時または対象 state の読み書き時に fail-closed で停止する。
一次診断には、path、observed version、supported version、復旧案を含む service log を使う。

SQLite 自体の健全性を切り分ける場合は両 writer を停止し、対象 DB に `PRAGMA quick_check` を実行する。
`reader_search.sqlite` は再生成可能な cache であり、runtime が破損を検出すると quarantine して再構築する。

標準 compose の named volume を診断する場合は、`shared-data-init` の一時 container へ SQLite CLI を導入して実行できる。
この一時 container での package 導入には、Alpine Linux の package repository へ接続できる環境が必要である。

```bash
set -euo pipefail

active_project=narou-viewer
compose=(docker compose -p "$active_project" -f docker-compose.prod.yml)

test -n "$("${compose[@]}" ps -a -q viewer-api)"
test -n "$("${compose[@]}" ps -a -q novel-fetcher)"
"${compose[@]}" stop viewer-api novel-fetcher

"${compose[@]}" run --rm --no-deps -T \
  --entrypoint sh \
  shared-data-init \
  -ceu '
    apk add --no-cache sqlite >/dev/null
    test -f /data/.shared-data-init-ok
    test -d /data/state
    test -d /data/novel-fetcher
    test -f /data/novel-fetcher/library.sqlite
    library_result="$(sqlite3 /data/novel-fetcher/library.sqlite "PRAGMA quick_check")"
    printf "library.sqlite: %s\n" "$library_result"
    test "$library_result" = ok
    if [ -f /data/state/ai_usage.sqlite ]; then
      usage_result="$(sqlite3 /data/state/ai_usage.sqlite "PRAGMA quick_check")"
      printf "ai_usage.sqlite: %s\n" "$usage_result"
      test "$usage_result" = ok
    fi
  '

"${compose[@]}" start novel-fetcher viewer-api
```

各 SQLite command が `ok` を返した場合だけ command は成功する。
`active_project` には現在の稼働系として記録した project 名を指定し、診断前後で同じ値を使う。

`ai_generation_settings.yaml` に非空の legacy `api_key` が残る場合、viewer-api は値を含めない warning を出す。
viewer-api は起動時にこの file を typed load するため、malformed document、未知 schema version、不正な crypto version、復号失敗は AI 機能だけでなく service 全体の起動を失敗させる。
master passphrase を設定すると、recognized document の起動時読込で encrypted payload への lazy migration を試みる。

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
