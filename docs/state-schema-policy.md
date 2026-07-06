# YAML state schema 互換ポリシー

この文書は `state/*.yaml` のうち、互換要件を明文化している `reading_state.yaml` の運用方針を固定する。現行 schema は [`architecture.md`](architecture.md) の「state YAML スキーマ」を参照する。

## 共通方針

- `state/*.yaml` は `viewer-api` が所有し、他サービスや frontend から直接書き換えない。
- `schema_version` は現行 `3` を維持する。互換読み込みの追加だけでは `schema_version` を上げない。
- 書き込みは `viewer-api` の repository / store 境界で最新読込、正規化、メモリ上更新、temp file、rename の順に行う。
- 読み込み互換は「既存ユーザーデータを読めること」を目的に限定する。新規保存では現行 schema の field だけを書き出す。
- 破損 YAML は暗黙に空 state へ置き換えず、読み込み error として返す。ファイル欠落だけは空 document として扱い、`Initialize()` で初期ファイルを作成する。

## `state/reading_state.yaml`

現行 schema:

```yaml
schema_version: 3
revision: 1
novels:
  "<novel_id>":
    last_read_episode_index: "120"
    position: 42
    state_version: 7
    updated_by_client_id: "reader-device-a"
    scroll:
      type: ratio
      value: 0.42
    updated_at: "2026-03-01T00:00:00.000Z"
```

### 読み込み互換として残す field

- `line_number`: 旧 position field として読み込み時の struct には残す。ただし既読位置の復元値には使わず、`position` がない場合は `position: 0` として扱う。
- `deleted`: 削除済み作品の tombstone として読み込む。tombstone は `last_read_episode_index`、`position`、`scroll`、`updated_at`、`updated_by_client_id` を復元しない。
- `state_version`: CAS と tombstone の世代として読み込む。負値や欠落は `0`、tombstone では最低 `1` に正規化する。
- `scroll`: `type: ratio` のみ読み込む。`value` は `0..1` へ clamp する。

### 書き出さない field

- `line_number`: 新規保存、CAS 更新、tombstone 更新のいずれでも書き出さない。
- `deleted` 以外の tombstone 付随情報: tombstone record には `state_version` と `deleted: true` だけを保存する。
- 空文字または空白だけの `updated_by_client_id`: `nil` として保存対象から外す。
- 不正な `last_read_episode_index`、不正な `scroll`: 正規化後に保存対象から外す。

### migration 方針

- 現時点では eager migration は行わない。互換 field を読めるようにしつつ、次回書き込み時に現行 schema で自然に再保存する。
- `line_number` から `position` への自動変換は行わない。旧 line 単位と現行 readerDocument 線形 position は意味が違うため、誤った復元より `position: 0` を優先する。
- `schema_version` が `3` 以外でも、現行 document shape として読める field は best-effort で正規化する。非互換 migration が必要になった時点で専用 migration を追加し、policy と architecture を更新する。

### CAS と tombstone

- `state_version` は作品単位の CAS 世代で、通常更新と tombstone 更新のどちらでも加算する。
- `ExpectedStateVersion` が現在の `state_version` と一致しない場合は書き込まず、現在 state を返して conflict とする。
- tombstone は stale client による復活を防ぐために保持する。削除済み作品を再 prune した場合も tombstone の `state_version` を加算する。

## Repository 境界

- `internal/state/readingstate.Repository` が `reading_state.yaml` の schema、正規化、read/write、CAS、tombstone を所有する。
- `internal/state/bookmarks.Repository` が `bookmarks.yaml` の schema、正規化、read/write、作品単位 prune を所有する。
- `internal/state/preferences.Repository` が `reader_preferences.yaml` の schema、既定値、正規化、read/write を所有する。
- `internal/state/novelsettings.Repository` が `novel_reader_settings.yaml` の schema、既定値、patch、作品単位 prune を所有する。
- 各 repository は public write method の入口で保存値を正規化し、同一 repository instance 内の read-modify-write を mutex で直列化する。
- `internal/store.Store` は既存 public API の facade として各 repository へ委譲し、削除済み state の確認や `PruneNovelState` の cross-domain coordination は当面 `Store` 側で扱う。
