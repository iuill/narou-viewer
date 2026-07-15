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

repair 後は同じ data tree を再走査した report を返します。正本側の finding が残る場合は recovery hint に従い、対応 build または同一 consistency group の backup を使って復旧します。
