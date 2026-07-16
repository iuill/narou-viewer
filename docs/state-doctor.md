# state doctor

`state-doctor` は `data/` 配下の viewer-api / novel-fetcher state を横断して診断する CLI です。既定は read-only dry-run で、YAML / JSON / SQLite の version、integrity、owner 間の対応関係、機微 file の配置と mode を確認します。

暗号化 cold backup / restore は [`state-backup.md`](state-backup.md) を参照してください。restore tooling は staging と公開後の両方で doctor を再利用します。

## 実行

repository root から human report を表示します。

```bash
bun run state:doctor --data-dir ./data
```

machine-readable report は `--format json` を使います。

```bash
bun run state:doctor --data-dir ./data --format json
```

`VIEWER_API_DATA_DIR` または `DATA_DIR` も利用できますが、誤った tree を診断しないよう運用手順では `--data-dir` の明示を推奨します。複数 file / WAL を含む一貫した時点を診断するときは、先に viewer-api と novel-fetcher の writer を停止してください。

exit code:

- `0`: warning / error finding なし
- `1`: warning または error finding あり
- `2`: 引数、data tree の open、report 出力、選択 repair の実行エラー

report の各 finding は `id`、`schema_id`、`path`、`kind`、`severity`、`observed`、`supported`、`recovery_hint` を持ちます。資格情報の値や作品本文は report に出力しません。

## 検査範囲

- viewer-api YAML / JSON の schema header と parse 可否
- `ai_usage.sqlite`、`reader_search.sqlite`、`novel-fetcher/library.sqlite` の version ledger と `PRAGMA quick_check`
- canonical episode の schema、DB row に対応する body file の欠落・hash mismatch、未参照 file
- extraction job / derived index mismatch
- character / term commit frontier inversion、profiles-only state
- reading / bookmarks が参照する library orphan
- `ai_generation_settings.yaml`、`ai_usage.sqlite`、`reader_search.sqlite` の `0600` と想定外配置
- `ai_generation_settings.yaml` の非空 legacy `api_key`。値自体は読取結果へ含めない

novel-fetcher の `schema_migrations` latest `3` と canonical episode version `1` は別 Go module の契約値を doctor 側にも持ちます。novel-fetcher の定数を変更する PR は doctor の契約値と fixture も同時に更新します。

## 限定 repair

自動 repair は再生成可能な派生 state / cache だけに限定します。正本、生成正本、監査履歴、未知の将来 version を normalize / delete しません。

1. dry-run report で `repair_kind` と finding ID を確認する。
2. 対象を個別に指定して実行する。

```bash
bun run state:doctor --data-dir ./data --apply --finding finding-0123456789abcdef
```

複数対象は `--finding` を繰り返します。`--apply` だけの実行、dry-run に存在しない古い ID、diagnostic-only finding は拒否します。現在の repair 対象は次だけです。

- job file を正本とする extraction job index の quarantine / rebuild
- current character events を正本とする derived character profile の quarantine / rebuild
- `reader_search.sqlite` の connection close、quarantine、新規 cache 作成。本文は通常アクセス時に lazy rebuild

`--apply` は最初の走査より前に viewer-api の writer lock を取得し、全 repair と再走査が終わるまで保持します。viewer-api が稼働中、または restore recovery journal が残る状態では mutation を開始せず拒否します。dry-run は lock を取得せず read-only のまま実行できます。

通常のscanは`.restore-staging-*` / `.restore-rollback-*`に似た名前も含めてdata tree全体の機微file配置を診断します。restore内部のpost-publish scanだけは、検証済みdurable journalが指す正確なtop-level staging / rollback pathとjournalを明示指定して除外します。名前prefixだけで任意directoryを除外しません。

YAML / JSONのcanonical stateとAI credential scanは、symlinkを辿らないnon-blocking open後に同じdescriptorでregular fileかを確認し、64 MiBを上限として読みます。FIFOやdeviceなどの特殊file、上限超過fileは待機・無制限読取せず`read_error`またはcredential scan errorとして報告します。

repair 後は同じ data tree を再走査した report を返します。正本側の finding が残る場合は recovery hint に従い、対応 build または同一 consistency group の backup を使って復旧します。

malformed extraction job / checkpoint は `novel_id` を安全に特定できないため、1 fileでも残る間は全作品の削除をfail-closedで停止します。malformed jobはさらにjob一覧・新規queue・起動時recoveryを停止し、checkpointは対象生成・対応jobのrecoveryでprovider呼出し前に停止します。まず両writerを停止してcold backupを確保し、対応buildまたはsupported backupで正本を復旧してください。手動で退避する場合は自動repairとして扱わず、元bytesを保持したうえで、そのjobの再実行・重複cost・関連checkpointを運用者が確認します。

## quarantine file の管理

`.unsupported-*`、`.corrupt-*`、`.rebuild-*` は自動削除せず、state doctorの通常inventoryにも含めません。特に`reader_search.sqlite.*`は第三者作品本文を含み得ます。current stateの再生成とbackup / restoreを確認し、両writerを停止した状態で、schemaごとの復旧価値を判断して機微fileとして削除します。checkpoint quarantineは重複provider requestの判断材料、character profile quarantineはeventsに存在しないheuristic情報を含み得るため、一律の期間削除は行いません。
