# 永続 state schema・互換・復旧ポリシー

この文書は、`data/` 配下の server state、`novel-fetcher` の保存データ、library export schema について、所有者、version、互換性、migration、復旧、backup / restore を横断管理する単一正本である。service とデータフローの責務分離は [`architecture.md`](architecture.md)、機能上の意味は各機能仕様を参照する。

registry の「現行」は 2026-07-15 時点の実装事実、「目標」はこの文書で採用する方針を表す。目標を未実装の安全性として扱わず、差分は follow-up Issue で追跡する。

## 1. 適用範囲と判断軸

### 1.1 所有境界

永続 schema の owner は 1 つにする。他 owner の保存ファイルを直接書き換えず、境界を跨ぐ更新は API または application service で調停する。

| owner | 所有範囲 |
| --- | --- |
| `viewer-api` | `state/` 配下の server state、AI 利用履歴、検索 cache |
| `novel-fetcher` | `novel-fetcher/library.sqlite`、`novel-fetcher/works/**`、将来の fetch task state |
| `viewer-web export` | 利用者が持ち出す library export document の producer contract。将来 importer を `viewer-api` に置いても交換 format は server 内部 schema と分離する |
| browser local | `localStorage` の端末依存設定と、Service Worker / Cache Storage の app-shell cache。server backup / restore の対象外 |

### 1.2 役割と復元可能性

| 役割 | 定義 |
| --- | --- |
| 正本 | 利用者入力、設定、取得済み library を表す通常運用の基準データ |
| 運用正本 | queue、job、status、進捗など、再開、重複防止、状態遷移の基準となるデータ |
| 生成正本 | AI 等で生成され、再生成が高コストまたは非決定的な履歴・event |
| 監査・履歴 | 機能の現在値を決めないが、失うと同じ履歴を再構成できない記録 |
| 派生 view | 別の正本または取得済み本文から意味的に再構築できる表示・索引 |
| cache | 正本から再構築でき、失っても correctness に影響しない高速化データ |
| 一時 state | 処理途中の checkpoint。破棄後の再実行で外部 API 呼出しや料金が再発生する場合がある |
| 交換 schema | export / import 用 document。server 内部 storage schema と独立して version 管理する |

正本か派生かだけで復旧方法を決めない。各 schema について、次のどれに当たるかを registry に記録する。

- ローカルで lossless に再構築可能
- ローカルで再計算可能だが、結果差または処理時間がある
- 外部再取得が必要で、取得元変更・削除により同一内容を保証できない
- 有償処理または外部副作用を伴う再実行が必要
- 再構築不能

### 1.3 互換性の4側面

schema の互換性は「新しい build が旧 data を読める」だけで判断しない。

1. backward read compatibility: 新しい build が旧 data を読める
2. forward read compatibility: 旧 build が新しい data を安全に読める
3. round-trip / write compatibility: 認識しない情報を保存し直しても失わない
4. operational compatibility: job、checkpoint、queue を誤解釈して外部処理や課金を重複させない

## 2. 共通 version・互換方針

### 2.1 version 軸

1 schema に複数の独立した version 軸が存在してよい。

- file document: `schema_version` または `schemaVersion`
- exchange document: `formatVersion`
- SQLite: `schema_migrations` または `PRAGMA user_version`
- encrypted payload: `api_key_version` 等の crypto format version
- prompt / checkpoint semantics: generation fingerprint または contract version

暗号方式変更と document field 変更は別々に判定し、crypto version を document schema version に統合しない。

### 2.2 version を上げる条件

次のいずれかに該当する場合は version を上げる。

- field の削除、rename、型・構造の変更
- required / optional、default、単位、意味、enum、状態遷移の変更
- 同じ値を旧 build が別の意味として解釈する変更
- 新 field を旧 writer が read-modify-write で消し、その消失が correctness、機密性、課金、rollback に影響する変更
- job、queue、checkpoint の解釈変更により二重実行、重複適用、課金再発生があり得る変更
- 正規化規則の変更で保存結果が不可逆に変わる変更
- migration が必要な変更

additive field の追加で version を維持できるのは、次をすべて満たす場合だけとする。

- field は optional で、欠落時の意味が安全かつ固定されている
- support 対象の旧 reader が未知 field を安全に無視できる
- support 対象の旧 writer が未知 field を保持するか、その document を書き戻さない
- field 消失が correctness、機密性、課金、rollback に影響しない
- 新旧 fixture の read と round-trip test がある

`term_profiles` の `description_facts` と character state の `identity_merge_events` を同一 version のまま追加した現状は、旧 writer による field 消失があり得る既存の rollback hazard であり、将来変更の標準例にはしない。

### 2.3 旧 version と migration

- 読込可能な旧 version を schema ごとに列挙する。
- field 欠落または version `0` を自動的に現行 version とみなさない。legacy として扱う場合は、その version と migration を registry に明記する。
- 旧 version の typed decode は version header を先に判定した後で行う。
- lazy re-save は明示的に support する旧 version からの migration に限る。
- migration は idempotent とし、同じ input に繰り返し適用しても意味が変わらないことを test する。

| 種別 | 標準 migration |
| --- | --- |
| singleton / per-novel の正本 YAML / JSON | header 判定後に version 別の明示 migrator を通す。認識済み旧 version のみ lazy re-save 可 |
| 生成正本 | 生成履歴や commit frontier を保持する専用 migration。暗黙の再生成を選ばない |
| SQLite 正本・監査履歴 | transaction 内の番号付き migration。未知の将来 migration は write 前に停止 |
| 派生 view / cache | quarantine または削除後に正本から rebuild |
| 運用正本 | schema migration と、旧 `running` を `interrupted` にする等の recovery transition を分離 |
| checkpoint | schema と generation fingerprint の双方を確認。不一致は auto-resume しない |
| 交換 schema | strict validator と version 別 importer。validation 完了前に mutation を開始しない |

### 2.4 未知の将来 version

| 役割 | read | write | recovery |
| --- | --- | --- | --- |
| 正本・生成正本 | 原則 error。検証済み read-only degraded mode だけ例外 | 必ず拒否し、元 bytes を変更しない | 対応 build、正式 migration、または backup を使う |
| 監査・履歴 | error または限定 read | append / update / prune を拒否 | 対応 build または migration。自動 drop しない |
| 運用正本 | `incompatible` として識別 | 自動状態遷移・自動再開を拒否 | 対応 build、migration、または明示的な破棄・再投入 |
| 派生 view・cache | 利用しない | 旧 file へ上書きしない | quarantine 後に正本から rebuild |
| 一時 checkpoint | auto-resume に使わない | 旧 checkpoint へ上書きしない | quarantine。重複課金・副作用を確認してから再実行 |
| 交換 schema | document 全体を拒否 | server state を変更しない | 対応 importer を使う |
| crypto payload | decrypt error | 再暗号化・消去しない | 対応実装と同じ secret を使う |

`reading_state.yaml` 等の現行実装が version を gate せず best-effort 正規化する事実は、registry の現行欄へ残す。ただし全 schema の標準にはせず、[#20](https://github.com/iuill/narou-viewer/issues/20) で write fence へ移行する。

### 2.5 欠落・破損・非対応 schema

- designated singleton state の欠落だけは新規環境として empty document を作成できる。
- 既存 file の parse error、I/O error、未知 version を欠落とみなさない。
- 正本、生成正本、監査・履歴を暗黙に empty state へ置換しない。
- cache / 派生 view は元 file を `.corrupt-*` または `.unsupported-*` として quarantine してから rebuild できる。
- 非対応 error は path、observed version、supported versions、推奨復旧を含める。
- unrelated schema が正常な場合に service 全体を止めるか、該当機能だけ unavailable にするかを schema ごとに決める。

### 2.6 atomicity・locking・multi-instance

| 境界 | 現行方式 | 注意点 |
| --- | --- | --- |
| core singleton YAML | `Store` と各 repository の mutex。最新読込、正規化、temp file、file fsync、rename、parent directory fsync | process 内のみ。typed struct は未知 field を保持しない |
| generated character / term | `novelstate.WithLock(novelID)` と atomic file write | events、profiles、terms の複数 file 更新は1 transactionではない |
| heuristic character profile | atomic file write。profile store 自体には novel 単位 lock がない | 同一作品・異なる境界の caller 間を store 単体では直列化しない |
| extraction job / index | process-wide `jobsMu`、一部 novel lock、atomic file write | job と index の crash 差分を許容し、index は job file から rebuild する |
| checkpoint | atomic JSON write。workflow が処理順を調停 | mismatch 後の再実行は外部 cost を伴い得る |
| `ai_usage.sqlite` | process-wide write mutex、SQLite transaction、`busy_timeout`、file mode `0600` | WAL は明示していない。複数 process writer は対象外 |
| `reader_search.sqlite` | open 初期化 mutex、max open connections 1、SQLite transaction、`busy_timeout`、file mode `0600` | rebuildable cache。WAL は明示していない |
| `novel-fetcher/library.sqlite` | SQLite transaction、WAL、foreign key、番号付き migration | DB と `works/**` は単一 transaction ではない |

multi-instance を許可する場合は process 内 mutex を前提にせず、外部 lock または transactional store へ移行する。

### 2.7 機微データ

- credentials、第三者本文、取得済み HTML / 画像、読書行動、model output、会話・tool I/O を含む schema を registry で明示する。
- AI credential の encrypted payload と master passphrase は別管理する。passphrase を state backup archive に平文同梱しない。
- backup archive、manifest、staging / temporary file 自体を機微データとして扱い、保存時・転送時の暗号化と最小権限のアクセス制御を適用する。詳細は 6.2 を参照する。
- API usage store は producer から受け取った snapshot をそのまま JSON 化し、credentials 系 key や内容の汎用 redaction は行わない。現行 producer は AI credential を含めない構造を組み立てるが、制限付き tool I/O にはユーザー文言や第三者作品本文が含まれ得る。producer を追加・変更するときは credential 非包含と保存対象を test するか、明示的な redaction を実装して test する。
- repository の fixture、test、docs には synthetic または利用許諾済みデータだけを使う。

## 3. 永続 schema registry

### 3.1 inventory・役割・復元性

path は `data/` からの相対 path を表す。

| ID | path / schema | owner / status | 役割 | 復元性・消失影響 | prune | 機微性 |
| --- | --- | --- | --- | --- | --- | --- |
| `VA-READING` | `state/reading_state.yaml` | viewer-api / 実装済み | 利用者正本 | 再構築不能。既読位置、CAS 世代、tombstone を失う | 物理削除せず tombstone 化 | 読書行動 |
| `VA-BOOKMARKS` | `state/bookmarks.yaml` | viewer-api / 実装済み | 利用者正本 | 再構築不能 | 作品単位で削除 | 読書行動、利用者 label |
| `VA-PREFERENCES` | `state/reader_preferences.yaml` | viewer-api / 実装済み | 利用者正本 | 元設定は再構築不能。既定値へ戻すことは可能 | 対象外 | 利用者設定 |
| `VA-NOVEL-SETTINGS` | `state/novel_reader_settings.yaml` | viewer-api / 実装済み | 利用者正本 | 元設定は再構築不能 | 作品単位で削除 | 利用者設定 |
| `VA-AI-SETTINGS` | `state/ai_generation_settings.yaml` | viewer-api / 実装済み | 設定正本 | profile / model は再構築不能。credential 復号には同じ passphrase が必要 | 対象外 | credentials、provider / model 設定 |
| `VA-PUBLICATIONS` | `state/publications.yaml` | viewer-api / 実装済み | 利用者・補完 metadata 正本 | override や選択状態は再構築不能 | 作品単位で削除 | 外部 publication metadata |
| `VA-CHAR-EVENTS` | `state/character_events/*.yaml` | viewer-api / 実装済み | AI 生成正本・commit frontier | 再生成は有償・非決定的。stable ID、merge、履歴を失う | 作品単位で削除 | model output、作品由来情報 |
| `VA-CHAR-PROFILES` | `state/character_profiles/*.yaml` | viewer-api / 実装済み | 派生 view | AI 部分は events から materialize 可。heuristic 部分は本文から再計算可能だが現行自動復旧は限定的 | 作品単位で削除 | model output、作品由来情報 |
| `VA-TERM-PROFILES` | `state/term_profiles/*.yaml` | viewer-api / 実装済み | AI 生成正本 | 再生成は有償・非決定的 | 作品単位で削除 | model output、作品由来情報 |
| `VA-EXTRACTION-JOBS` | `state/extraction_jobs/*.yaml` | viewer-api / 実装済み | 運用正本・job 履歴 | 再構築不能。破棄・再投入は重複課金や重複適用の判断が必要 | 作品単位で削除 | model / profile、error、進捗 |
| `VA-EXTRACTION-INDEX` | `state/extraction_jobs/index/*.yaml` | viewer-api / 実装済み | 派生 index | job file から rebuild 可 | 作品単位で削除 | job ID、進捗 |
| `VA-EXTRACTION-CHECKPOINT` | `state/extraction_jobs/checkpoints/*.json` | viewer-api / 実装済み | 一時 state | 再実行できるが provider request と料金が再発生し得る | commit 後と作品削除時に削除 | 未commit model output |
| `VA-AI-USAGE` | `state/ai_usage.sqlite` | viewer-api / 実装済み | 監査・利用履歴 | 再構築不能。消失しても現在の reader / generation state は壊れないが履歴を失う | 作品紐づき run を削除 | 利用 metadata、会話件数・文字数、転記されたユーザー文言、本文 excerpt / snippet / passage を含み得る制限付き tool I/O |
| `VA-READER-SEARCH` | `state/reader_search.sqlite` | viewer-api / 実装済み | 再生成可能 cache | canonical episode と reader document から lazy rebuild 可 | 作品行を削除 | 第三者作品本文の plain text |
| `NF-LIBRARY` | `novel-fetcher/library.sqlite` | novel-fetcher / 実装済み | library catalog・索引・取得状態の正本 | `works/**` と一体で保護。DB または file 単独 restore は不整合要因 | fetcher の作品削除で処理 | 作品 metadata、取得履歴 |
| `NF-CANONICAL-EPISODE` | `novel-fetcher/works/**/episodes/*.json` | novel-fetcher / 実装済み | 取得済み本文の local canonical copy | 再取得できても削除・改稿により同一内容を保証できない | 作品削除の `withFiles: true` で削除。`false` では残る | 第三者作品本文 |
| `NF-RAW-EPISODE` | `novel-fetcher/works/**/raw/episodes/*.html` | novel-fetcher / 実装済み | raw source snapshot / cache | best-effort 再取得。履歴的同一性は保証できない | 作品削除の `withFiles: true` で削除。`false` では残る | 第三者作品本文、元 HTML |
| `NF-ASSETS` | `novel-fetcher/works/**/assets/**` | novel-fetcher / 実装済み | 取得 asset | best-effort 再取得。元消失・差替えの可能性あり | 作品削除の `withFiles: true` で削除。`false` では残る | 第三者画像等 |
| `NF-TASKS` | path 未決定 | novel-fetcher / 予約（[#15](https://github.com/iuill/narou-viewer/issues/15)） | 将来の運用正本 | queue 順序、利用者意図、resume / cancel / idempotency を保持する | #15 で定義 | 対象 URL、option、error、進捗 |
| `EX-LIBRARY-V1` | library export YAML | viewer-web export / producer 実装済み、import 未実装 | 交換 schema | 利用者管理の export。server 全体 backup ではない | server prune 対象外 | 読書行動、栞、作品一覧 |
| `BROWSER-PREFERENCES` | `localStorage` の `narou-viewer.reader-local-preferences.v1` | browser / server registry 対象外 | 端末設定の正本 | 消失時は既定値へ戻り、元設定は再構築不能 | 利用者による browser storage 消去 | 端末設定 |
| `BROWSER-APP-SHELL` | SW / Cache Storage の `narou-viewer-shell-*` | browser / server registry 対象外 | app-shell cache | `/` と manifest を再取得して再構築可能 | SW activate / browser eviction | 第三者本文を含まない app shell |

### 3.2 version・現行挙動・目標

| ID | 現在の version 軸 | 現行挙動 | 目標 / recovery |
| --- | --- | --- | --- |
| `VA-READING` | `schema_version: 3` | version を gate せず現行 struct で正規化し、次回 write で現行 shape に保存 | 明示旧 version のみ migration。未知将来 version は write fence（[#20](https://github.com/iuill/narou-viewer/issues/20)） |
| `VA-BOOKMARKS` | `schema_version: 3` | version を検査せず正規化。未知 field は write で失われ得る | 明示旧 version のみ読込。未知将来 version は mutation / prune を拒否（#20） |
| `VA-PREFERENCES` | `schema_version: 3` | version を検査せず既定値補完 | 未知 version を既定値へ暗黙変換しない（#20） |
| `VA-NOVEL-SETTINGS` | `schema_version: 3` | version を検査せず既定値補完 | 未知 version を prune で上書きしない（#20） |
| `VA-AI-SETTINGS` | document `schema_version: 2`、credential `api_key_version: 1` | document version は gate しない。平文 key は passphrase があれば encrypted v1 へ lazy migration。未知 crypto version は decrypt error | document と crypto を別軸で strict 判定。未知 payload を消去・再保存しない（#20） |
| `VA-PUBLICATIONS` | `schema_version: 1` | `0` は `1` に補完。他の値は拒否しない | field なし / `0` を legacy とするなら明記し、未知将来 version は write / prune fence（#20） |
| `VA-CHAR-EVENTS` | `schema_version: 1` | load の strict guard なし。events 欠落時は legacy profile migration path がある | 未知将来 version では生成、materialize、prune を拒否（#20） |
| `VA-CHAR-PROFILES` | runtime は version field なし。一部 E2E fixture に `schema_version: 2` | runtime は fixture の version / revision / updated_at を decode しない | derived view `schema_version: 1` を正式導入し、field なしを legacy v0 として rebuild / migrate（#20） |
| `VA-TERM-PROFILES` | `schema_version: 1` | load は version を gate しない。`description_facts` 追加後も v1 | 未知将来 version は write fence。同一 v1 内の rollback hazard を解消または明示維持（#20） |
| `VA-EXTRACTION-JOBS` | `schema_version: 2` | version を gate せず、parse 不可 file は log して skip。起動時は旧 `running` を `queued` にする | 未知 version は `incompatible` とし auto queue / resume しない。状態機械は [#16](https://github.com/iuill/narou-viewer/issues/16) |
| `VA-EXTRACTION-INDEX` | `schema_version: 2` | version を gate せず上書き時に v2 化 | mismatch / corruption 時に job file から rebuild（#20） |
| `VA-EXTRACTION-CHECKPOINT` | `schemaVersion: 4` + generation fingerprint | schema、novel、boundary、fingerprint 不一致または read error は empty checkpoint として先頭から再実行 | mismatch は auto-resume せず quarantine。重複 request / cost を #16 で保護 |
| `VA-AI-USAGE` | version 管理なし | `CREATE TABLE IF NOT EXISTS` と一部 column fallback。transaction と process 内 write mutex。WAL は明示しない | 番号付き migration と future-version guard。自動 drop しない（[#21](https://github.com/iuill/narou-viewer/issues/21)） |
| `VA-READER-SEARCH` | version 管理なし | `CREATE TABLE IF NOT EXISTS`、transaction、single connection、`busy_timeout`。WAL は明示しない | cache version を導入し、close、quarantine / drop、rebuild（[#22](https://github.com/iuill/narou-viewer/issues/22)） |
| `NF-LIBRARY` | `schema_migrations`、既知 latest `3` | 番号付き migration、transaction、WAL、foreign key、incremental auto-vacuum。未知将来 migration の明示 guard なし | supported latest 超過は startup write 前に停止（[#23](https://github.com/iuill/narou-viewer/issues/23)） |
| `NF-CANONICAL-EPISODE` | JSON `schema_version: 1` | write は v1。read は typed unmarshal 後に version を検査しない | header を先に検査し、未知 version を本文として返却・再保存しない（#23） |
| `NF-RAW-EPISODE` | version なし | opaque HTML | schema migration 対象外。DB metadata、source URL、hash で管理 |
| `NF-ASSETS` | file format 固有、索引は DB | opaque binary | schema migration 対象外。DB metadata と hash の整合を検査 |
| `NF-TASKS` | 未定 | queue は memory only | #15 で version、状態遷移、idempotency、queue order、起動 recovery、未知 version 停止規則を定義 |
| `EX-LIBRARY-V1` | `formatVersion: 1` | producer が YAML を生成。reader state 取得失敗は warning とし部分 export を作れる。import なし | unknown version / field / malformed data を mutation 前に strict reject。dry-run と apply は同一 validator（[#17](https://github.com/iuill/narou-viewer/issues/17)） |

## 4. schema 別の重要事項

### 4.1 reader state、bookmarks、preferences

`reading_state.yaml` の現行 shape:

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

- `state_version` は作品単位の CAS / tombstone 世代であり、document `schema_version` とは別である。
- `revision` は file 書換えごとに加算する document 内部世代であり、作品単位の `state_version` とは別である。
- `ExpectedStateVersion` が現在の `state_version` と一致しない場合は書き込まず、現在 state と conflict を返す。
- 旧 `line_number` は decode するが位置へ変換せず、`position` 欠落時は `0` とする。旧 line 単位と reader document の線形 position は意味が異なるためであり、新規保存、CAS 更新、tombstone 更新では `line_number` を書き出さない。
- `deleted` tombstone は stale client による復活を防ぐため物理削除しない。意味上の情報は `state_version` と `deleted: true` に限定し、既読位置、scroll、更新日時、client ID は復元しない。再 prune でも世代を加算する。
- `scroll.type` は `ratio` だけを認識し、値を `0..1` に clamp する。
- 空文字または空白だけの `updated_by_client_id`、不正な `last_read_episode_index` / `scroll` は正規化後の値として保持しない。
- 破損 YAML を empty document に置換せず、missing file だけ初期化できる。
- bookmarks、preferences、novel reader settings にも同じ unknown-version write fence 原則を適用する。

### 4.2 AI settings と暗号化 credential

- document schema version と `api_key_version` を独立管理する。
- encrypted credential の restore には同じ `AI_GENERATION_SETTINGS_MASTER_PASSPHRASE` が必要である。
- 現行 document は legacy 互換用の平文 `api_key` field を読み込む。master passphrase がない場合は暗号化 migration を行わないため、既存 file に平文 credential が残り、互換経路から使用され得る。
- backup manifest に記録する secret / key 情報は identifier または reference に限定し、secret 値、平文 / derived key、private recovery material、平文断片を記録しない。salt、nonce、tag、wrapped data key 等の暗号 format に必要な metadata は authenticated archive header に保持できるが、manifest へ重複記録しない。
- unknown crypto version、passphrase 不足、認証失敗を「key なし」に正規化しない。
- plaintext から encrypted payload への lazy migration は recognized document にだけ適用する。

### 4.3 character / term / extraction state

- character events は AI 生成人物 state の正本、character profiles は表示用 derived view である。
- heuristic profile が events より新しい場合があるため、events materialize で無条件に巻き戻さない。
- generated save は events を先、profiles を後に書く。途中 crash では events を正本として profiles を rebuild する。
- extraction pipeline は term profile を先に保存し、character frontier を commit marker とする。term が先行した partial write は frontier より未来を公開しない。
- profiles-only restore は禁止する。現行 legacy migration が profile を events の seed として扱う可能性があるため、対応 events を restore するか profile を削除する。
- job file は cache ではなく、利用者に見せる状態と重複実行防止の運用正本である。unreadable / unknown job を黙って一覧から消さない。
- checkpoint mismatch は correctness 上再計算できても、既に発生した provider request と cost を戻せない。単に「破棄・再実行が安全」としない。
- #16 では `queued`、`running`、`pausing`、`paused`、`interrupted`、`canceled`、`succeeded`、`failed` の状態遷移と version を同時に定義する。

抽出固有の保存順、公開境界、retry は [`extraction.md`](extraction.md) を参照する。

### 4.4 viewer-api SQLite

`ai_usage.sqlite` は reader state や AI settings の正本ではないが、producer が投入した token / cost 等の利用情報、request 分類、tool I/O、分析用 snapshot は再生成不能である。現行の読書AI snapshot は `message` / `history` / `answer` の本文を会話用の専用 field としては保存せず、件数・文字数を保存する。ただし、件数・深さ・文字列長を制限した tool request / result には、モデルが転記したユーザー文言・検索語や、作品本文の excerpt / snippet / passage 等が含まれ得る。この制限処理は内容の redaction ではなく、usage store に key 名・内容ベースの汎用 redaction はない。schema mismatch で drop / rebuild せず、[#21](https://github.com/iuill/narou-viewer/issues/21) で transactional migration、snapshot の credential 非包含、ユーザー文言・作品本文の保持契約を整備する。

`reader_search.sqlite` は correctness に影響しない cache である。normalization contract または schema の mismatch、破損時は DB connection を close し、旧 DB を quarantine または削除してから lazy rebuild する。実装は [#22](https://github.com/iuill/narou-viewer/issues/22) で追跡する。

### 4.5 novel-fetcher storage

- `library.sqlite` と `works/**` は1つの logical consistency group である。
- `library.sqlite` は WAL と番号付き migration を使う。未知の将来 migration を検出した場合は既知 migration の適用前に停止する。
- canonical episode JSON は read 時にも `schema_version` を検証する。
- raw HTML と asset は再取得可能な場合があっても同一 bytes を保証できないため、履歴保存を重視する backup から自動除外しない。
- #15 の task state は novel-fetcher owner 内に置き、work state との照合規則を registry に追加する。

### 4.6 library export

現行 `formatVersion: 1` export は export timestamp、件数、warning、作品識別・metadata、reading state、bookmarks を含む。本文、asset、AI 生成 state、AI settings、AI usage、server cache は含まないため、server backup / restore と呼ばない。

Issue #17 の importer は strict shape / version validation、unknown field 拒否、dry-run、作品照合、位置正規化、bookmark conflict policy、zero-mutation failure、途中 failure の rollback、semantic round-trip test を満たす。

## 5. prune と reconciliation

### 5.1 現行

作品削除成功後、`viewer-api` は現在次の順で処理する。

1. reading state を tombstone 化し、bookmarks と novel reader settings を削除
2. character profile、character events、term profile、extraction job index、job、checkpoint をこの順で削除
3. publications の作品 entry を削除
4. `ai_usage.sqlite` の作品紐づき run / request / snapshot を削除
5. `reader_search.sqlite` の作品紐づき row を削除

reader preferences と AI settings は作品非依存なので prune しない。複数 file / DB を跨ぐ全体 transaction はなく、途中 error では処理を停止するが先行変更を rollback しない。

- character profile / events、term profile、job index は path で特定し、schema header や内容を読まず直接削除するため、unreadable file や未知の将来 version も保護しない。
- extraction job は YAML parse error を log して skip するが、parse できて `novel_id` が一致すれば `schema_version` を検査せず削除する。checkpoint は JSON parse error を skip し、parse できて `novelId` が一致すれば `schemaVersion` を検査せず削除する。
- reading、bookmarks、novel settings、publications は parse error で停止する一方、将来 version の guard はない。parse 可能な未知 version を対象 mutation で現行 shape に再保存し、未知 field を失う可能性がある。`ai_usage.sqlite` の prune にも current schema version guard はない。
- 再 prune では reading tombstone の世代を加算し、他の既削除 state は概ね no-op になる。partial failure の orphan を自動診断する経路は現行未実装である。

### 5.2 目標（#20・#21・#22）

- viewer-api file state は typed decode、mutation、delete より先に schema header を判定する。未知の将来 version では prune と lazy migration を拒否し、元 bytes を変更しない（[#20](https://github.com/iuill/narou-viewer/issues/20)）。
- unreadable な正本・生成正本は削除せず、missing、parse error、unknown version を区別して復旧案を報告する。派生 view / cache は registry が rebuild を認める場合だけ quarantine または削除できる。
- `ai_usage.sqlite` は future-version guard が prune にも適用されるまで自動 DELETE を安全な保証としない（[#21](https://github.com/iuill/narou-viewer/issues/21)）。`reader_search.sqlite` は version mismatch を検出したら作品単位 mutation ではなく安全な cache rebuild を選べるようにする（[#22](https://github.com/iuill/narou-viewer/issues/22)）。
- reading tombstone と既存の prune 順序を維持しつつ、繰り返し実行しても削除対象が復活せず収束すること、unknown version の元データを変更しないことを test する。

### 5.3 診断（#24・未実装）

state doctor は orphan、job / index mismatch、frontier inversion、parse error、unknown version を read-only 既定で報告し、明示 apply なしに state を変更しない。この診断は現行 prune の保証ではなく、[#24](https://github.com/iuill/narou-viewer/issues/24) で実装する。

## 6. backup・restore・rollback

### 6.1 consistency group

| group | 内容 | 要件 |
| --- | --- | --- |
| `NF-CANONICAL` | `novel-fetcher/library.sqlite` + `novel-fetcher/works/**` | 同一 consistency point で取得 |
| `VA-CORE` | reading、bookmarks、preferences、novel settings、AI settings、publications | 利用者 state と設定として保護 |
| `VA-EXTRACTION` | character events、term profiles、必要に応じて jobs / checkpoints。profiles / index は derived | frontier と job / checkpoint を同一 snapshot にする |
| `VA-HISTORY` | `ai_usage.sqlite` | 履歴保持が必要なら必須。cache として除外しない |
| `VA-CACHE` | character profiles、job index、reader search 等 | 省略可。省略を manifest に記録 |
| `SECRETS` | AI settings master passphrase 等 | backup archive と別の secret management |

full backup では `state/` と `novel-fetcher/` を同じ snapshot generation として取得する。「同じ時刻に copy を開始した」だけでは一貫性を保証しない。archive は encrypted credential、平文の legacy credential、第三者作品本文・HTML・画像、読書行動、model output、tool I/O、AI usage history を含み得るため、passphrase を別管理するだけで安全とはみなさない。

### 6.2 snapshot・archive の標準

優先順は次の通り。

1. `viewer-api` と `novel-fetcher` の writer を quiesce / stop し、open file を close して copy
2. SQLite online backup API と file-tree snapshot を application-level barrier で組み合わせる
3. filesystem / volume の atomic snapshot を両 service の write barrier と組み合わせる

WAL DB の main DB、`-wal`、`-shm` を稼働中に別々に順次 copy する手順を標準にしない。`-shm` は restore payload の必須要素として扱わない。viewer-api SQLite が WAL を明示していない場合も、稼働中の raw file copy を安全と仮定しない。tooling は [#25](https://github.com/iuill/narou-viewer/issues/25) で追跡する。

- archive、manifest、staging / temporary file は AEAD 等の改ざん検知付き archive encryption または同等の storage-level encryption で保存し、転送は認証付き暗号化経路に限定する。local file は `0600`、local directory は `0700` を基準とし、remote object / volume snapshot / KMS は同等の最小権限 ACL / IAM で生成・保管・restore の主体を制限する。
- 平文の backup 暗号鍵 / unwrapped data encryption key、KMS の private key material、AI settings master passphrase を archive、manifest、log に含めず、相互に流用しない。manifest に記録する secret / key 情報は identifier / reference に限定し、secret 値、平文 / derived key、private recovery material、平文断片を記録しない。salt、nonce、tag、wrapped data key 等の暗号 format に必要な metadata は authenticated archive header に保持できるが、manifest へ重複記録しない。manifest を archive 外に置く場合も同等に暗号化・アクセス制御する。
- snapshot 確定前かつ writer barrier 下で `ai_generation_settings.yaml` の raw YAML を read-only 検査する。非空の legacy `api_key` があれば、recognized schema を対応 build と master passphrase で暗号化 migration し、再検査で非空の平文値がないことを確認するまで backup を拒否する。未知 schema を typed load で正規化・再保存してはならず、[#20](https://github.com/iuill/narou-viewer/issues/20) の version guard を前提とする。
- backup manifest には timestamp、snapshot generation ID、application build、schema ID と observed / supported version、含めた / 省略した group、hash または snapshot ID、必要な secret の identifier / reference、snapshot method を記録する。
- retention 期間と保持世代数、rotation、期限切れ archive・remote object version・volume snapshot の削除を定義し、restore 可能な世代の backup key / KMS key lifecycle と同期する。生成失敗・cancel 時の partial archive / staging、restore 時の復号 temporary file は公開・retention 対象にせず削除し、tooling 管理下の path / object / version から参照不能になったことを確認する。一般 filesystem、COW、SSD の物理 secure erase は保証せず、可能なら平文 staging を作らない stream encryption と世代固有鍵の cryptographic erasure を使い、secret を log に出さない。

### 6.3 restore 順序

1. writer を停止する。
2. `NF-CANONICAL` を restore する。
3. `VA-CORE`、`VA-EXTRACTION`、`VA-HISTORY` を restore する。
4. snapshot に含まれない derived view / cache / index を削除する。
5. 対応 build で startup migration を実行する。
6. schema version、DB integrity、frontier、orphan state を検査する。
7. service を公開する。

unsupported future version を current build で強制 normalize しない。

### 6.4 部分 restore と rollback

| restore 内容 | 可否・手順 |
| --- | --- |
| character events のみ | 可。profiles を削除し events から materialize |
| character profiles のみ | 不可。対応 events を restore するか profile を削除 |
| term profiles のみ | 条件付き。character frontier より未来は非公開。同じ snapshot の events を推奨 |
| extraction jobs / checkpoints のみ | 原則不可。生成正本、fingerprint、frontier と同じ snapshot でなければ auto-resume しない |
| reader search のみ | 不要。削除して lazy rebuild |
| reading / bookmarks のみ | 条件付き。novel / episode の存在を検査し orphan を報告 |
| `library.sqlite` のみ | 不可。`works/**` と同じ group で restore |
| `works/**` のみ | 不可。DB 索引、ID、取得状態と不整合になる |
| AI settings のみ | 可。同じ master passphrase が必要。復号不能時に key を消去しない |
| AI usage のみ | 可。novel ID orphan を許容するか、restore 後に明示 prune |

arbitrary downgrade は保証しない。rollback 前に full snapshot を取得し、旧 writer が新 field を落とす schema を退避する。unknown-future-version write fence がない旧 build を state writer として起動しない。

## 7. schema 変更 PR の完了条件

永続 schema を変更する PR は、該当する項目を満たす。

- [ ] registry の該当 ID を同一 PR で更新した
- [ ] 現行 version と変更後 version を明記した
- [ ] version を上げる / 上げない理由を round-trip compatibility を含めて説明した
- [ ] support する旧 version と未知将来 version の read / write / recovery を定義した
- [ ] unknown future version で元 file / DB を変更しない test がある
- [ ] 旧 fixture の read test と current writer の golden fixture または semantic round-trip test がある
- [ ] migration の idempotency test がある
- [ ] job / checkpoint / queue なら重複外部呼出し、重複課金、重複適用の test がある
- [ ] multi-file commit の crash point と recovery を説明した
- [ ] prune の対象、繰り返し安全性、reading tombstone の世代加算を test した
- [ ] rollback 可否と必要な退避手順を記載した
- [ ] credentials、第三者本文、model output、AI usage snapshot の file mode / credential 非包含または redaction / backup を確認した
- [ ] backup / restore に影響する場合、archive / manifest / temporary file の暗号化、アクセス制御、legacy plaintext credential 検査、retention、失敗時 cleanup を確認した
- [ ] `architecture.md` と機能仕様に矛盾しないことを確認した
- [ ] export / import なら malformed、unknown version / field、dry-run、zero-mutation failure を test した

## 8. follow-up

| Issue | gap |
| --- | --- |
| [#15](https://github.com/iuill/narou-viewer/issues/15) | novel-fetcher task state の path、version、状態遷移、idempotency、recovery |
| [#16](https://github.com/iuill/narou-viewer/issues/16) | extraction job / checkpoint の状態機械、incompatible state、再実行 cost 保護 |
| [#17](https://github.com/iuill/narou-viewer/issues/17) | `EX-LIBRARY-V1` strict importer、dry-run、atomic apply、conflict policy |
| [#20](https://github.com/iuill/narou-viewer/issues/20) | viewer-api file state の version guard、write fence、fixture / migration test、character profile schema |
| [#21](https://github.com/iuill/narou-viewer/issues/21) | `ai_usage.sqlite` の番号付き migration、future-version guard、snapshot の credential・ユーザー文言・作品本文保持契約 |
| [#22](https://github.com/iuill/narou-viewer/issues/22) | `reader_search.sqlite` の cache version と安全な rebuild |
| [#23](https://github.com/iuill/narou-viewer/issues/23) | novel-fetcher DB / canonical episode の future-version guard |
| [#24](https://github.com/iuill/narou-viewer/issues/24) | #20・#21・#23 の schema identification / version contract を使う state doctor / reconciliation command |
| [#25](https://github.com/iuill/narou-viewer/issues/25) | #20・#21・#23 を前提とする consistent backup / restore tooling、archive 機密性、manifest、lifecycle |
