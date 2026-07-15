# state backup / restore

`state-backup` は viewer-api と novel-fetcher の永続 state を、1 つの cold snapshot generation として取得・復元する CLI です。`NF-CANONICAL`、`VA-CORE`、`VA-EXTRACTION`、`VA-HISTORY` を同じ generation に含め、再生成可能な `VA-CACHE` と archive 外で管理する `SECRETS` は除外します。

archive は gzip 圧縮した tar stream を [age](https://age-encryption.org/) v1 で暗号化した `.tar.gz.age` です。manifest も archive 内にあり、平文 staging file を作らずに backup します。archive は第三者作品本文・画像、読書履歴、AI 利用履歴、暗号化済み credential を含み得る機微データです。

## writer の停止

現行 viewer-api / novel-fetcher は、それぞれ `state/.viewer-api-writer.lock` と `novel-fetcher/.novel-fetcher-writer.lock` を process lifetime 中保持します。backup / restore は両方の lock を取得できなければ fail-closed で停止します。

production compose では先に writer を停止します。

```bash
docker compose -f docker-compose.prod.yml stop viewer-api novel-fetcher
```

lock 導入前の古い build は lock を保持しません。古い build から移行する最初の backup では、`docker compose ps` などでも両 writer の停止を別途確認してください。稼働中 SQLite の main DB / WAL や file tree を個別に raw copy する運用はサポートしません。

## backup key

自動運用では backup 専用の age X25519 または hybrid recipient を推奨します。private identity は `0600` の別 file / secret management に置き、AI settings の master passphrase と流用しません。`--key-reference` は rotation 世代を識別する非 secret の ID だけにします。

手動運用では `--passphrase-file` も利用できます。passphrase 自体ではなく、末尾改行を除いた内容を読む `0600` の regular file を指定します。passphrase、private identity、unwrapped key は command line、archive、manifest、log に含めません。

## backup

repository checkout から実行する場合:

```bash
mkdir -m 700 ./backups
bun run state:backup backup \
  --data-dir ./data \
  --output-dir ./backups \
  --recipient 'age1...' \
  --key-reference 'local-x25519-2026q3' \
  --build 'self-host-2026q3'
```

production image には `state-backup` と `state-doctor` も含まれます。停止済み compose volume に対して one-shot container を使う例:

```bash
docker compose -f docker-compose.prod.yml run --rm --no-deps \
  --entrypoint state-backup \
  -v "$(pwd)/backups:/backups" \
  viewer-api \
  backup \
  --data-dir /data \
  --output-dir /backups \
  --recipient 'age1...' \
  --key-reference 'local-x25519-2026q3' \
  --build 'self-host-2026q3'
```

backup 前に次を実施します。

- 両 writer lock の排他取得
- `ai_generation_settings.yaml` の raw YAML に非空 legacy `api_key` がないことの確認
- state doctor による schema / SQLite / canonical file / frontier / 機微 file mode の preflight
- symlink と non-regular payload の拒否

archive は一時 `.partial` を `0600` で作成し、age stream、gzip、tar をすべて close / sync できた後だけ最終名へ no-replace で公開します。失敗・cancel 時の partial は削除します。出力 directory は `0700`、archive は `0600` に固定します。

## manifest と consistency group

暗号化 archive 内の `manifest.json` は次を記録します。

- timestamp、generation ID、application build、snapshot method
- schema ID、path、observed / supported version、status、group
- payload file の path、group、size、mode、SHA-256
- 含めた / 除外した group
- backup key と AI settings master passphrase の非 secret reference
- backup 前 state doctor の件数 summary

既定 group:

| group | backup 対象 |
| --- | --- |
| `NF-CANONICAL` | `library.sqlite`、停止後に残る WAL、`works/**` |
| `VA-CORE` | reading、bookmarks、preferences、novel settings、AI settings、publications |
| `VA-EXTRACTION` | character events、term profiles、job、checkpoint |
| `VA-HISTORY` | `ai_usage.sqlite` と停止後に残る rollback journal |
| `VA-CACHE` | 除外。character profiles、job index、reader search cache は復元後に再生成 |
| `SECRETS` | 除外。backup identity / passphrase と AI settings master passphrase は別管理 |

## restore

restore は full generation だけを受け付けます。必須 group の欠落、未知 manifest、現在の build が読めない schema version、hash / mode / group mismatch、改ざんされた age archive を payload 公開前に拒否します。

```bash
bun run state:backup restore \
  --data-dir ./data \
  --archive ./backups/narou-viewer-...tar.gz.age \
  --identity-file /secure/backup-identity.txt \
  --key-reference 'local-x25519-2026q3'
```

restore は次の順で処理します。

1. archive を復号し、manifest、全 payload hash、group、schema compatibility を read-only preflight する。
2. 両 writer lock を取得する。
3. `data/.restore-staging-<generation>` を `0700` で作り、2 回目の復号で payload を `0600` 相当の元 modeへ staging する。
4. staging tree に state doctor を実行する。
5. `NF-CANONICAL`、`VA-CORE`、`VA-EXTRACTION`、`VA-HISTORY` の順に同一 filesystem 内で publish する。
6. archive に含まれない character profile、job index、reader search cache を削除する。
7. live tree に state doctor を実行し、error finding があれば publish 前 generation へ rollback する。
8. staging / rollback tree を削除し、最終 report を返す。

restore staging は復号済み機微データです。tool は成功・失敗時に管理 path から削除しますが、一般 filesystem、COW、SSD の物理 secure erase は保証しません。backup は平文 staging を作らず、restore staging の寿命だけを短くしています。

restore 後、対応 build を一度起動して supported startup migration と derived state の lazy rebuild を行い、再停止して state doctor を実行します。

```bash
docker compose -f docker-compose.prod.yml start novel-fetcher viewer-api

# migration / health 確認後、必要なら再停止して診断
docker compose -f docker-compose.prod.yml stop viewer-api novel-fetcher
docker compose -f docker-compose.prod.yml run --rm --no-deps \
  --entrypoint state-doctor viewer-api \
  --data-dir /data
```

warning は orphan や derived index の lazy rebuild 待ちを含み得ます。error finding が残る generation は公開せず、対応 build または別の supported archive を使います。

## retention と rotation

backup 成功後、既定で newest 7 世代を必ず残し、それ以外で 30 日を超えた local archive を削除します。

```bash
bun run state:backup prune \
  --output-dir ./backups \
  --keep 7 \
  --max-age 720h
```

remote object version、volume snapshot、KMS lifecycle は v1 tooling の対象外です。外部保管先でも authenticated encryption と最小権限 ACL / IAM を維持し、retention 内の全 archive を復号できる key generation を残します。key rotation では新 key で取得した restore test 済み generation を確保してから、旧 archive と旧 key を同じ lifecycle で失効させます。
