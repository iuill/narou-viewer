# キャラクター一覧生成機能

この文書は、作品ごとのキャラクター一覧生成と表示に関する現行仕様をまとめる。全体の責務分離とデータ境界は [`architecture.md`](architecture.md) を優先する。

## 目的

- 本文読書中に「この人物は誰だったか」をすぐ確認できるようにする。
- キャラクター名、別名、概要、容姿、性格、重要度を表示する。
- 未読話の情報を混ぜず、読書体験を壊さない。
- 生成処理は読書 API を塞がない非同期ジョブとして扱う。

## 導線

- 本文表示画面の reader panel から「キャラクター一覧」を開く。
- 未生成の場合は、生成ダイアログからキャラクター一覧生成を依頼できる。
- 生成中も本文読書は継続できる。
- 第1話では、現在話の前話までという既定境界を作れないため、本文画面からの生成は行わない。

## ネタバレ境界

- 境界は `episodeIndex` を基準にする。
- 本文画面から生成するときの既定 `upToEpisodeIndex` は、現在話の前話。
- 一覧取得時は `upToEpisodeIndex` 以下で本文に明示された情報だけを返す。
- LLM には推測、補完、伏線からの先読みを保存させない。
- キャラクター一覧や読書AIから参照する snapshot でも、境界外の情報は返さない。

## 保存方針

- 保存先は `viewer-api` 管理領域の `data/state/` 配下とする。
- `.narou/*`、`小説データ/*`、`novel-fetcher/works/*` には書き込まない。
- キャラクター情報は、固定項目と履歴型項目に分ける。

固定項目:

- `canonicalName`
- `fullName`
- `gender`
- `firstAppearanceEpisodeIndex`
- `aliases`

履歴型項目:

- `appearance`
- `personality`
- `summary`

履歴型項目は、それぞれ `episodeIndex` と表示用完成文を持つ。UI へ返すときは、`upToEpisodeIndex` 以下で最新の1件を選ぶ。

## 生成方式

- `viewer-api` の internal AI module が OpenRouter 経由で LLM を呼び出す。
- 保存済み AI プロファイルの `modelId` を使い、構造化出力の安定性を優先する。
- 作品全量を一度に渡さず、対象範囲を複数話バッチに分ける。
- 長い話は必要に応じて話内 chunk に分ける。
- 初回抽出で既存候補がない場合は、本文に明示的に登場する人物を `newCharacters` として返すよう prompt と payload の両方で指示する。
- 抽出結果は話単位の事実として正規化し、名前または alias の厳密一致を中心に保守的に統合する。同じ LLM 応答内の新規人物重複は、安定 ID を採番する前に統合する。
- `aliases` は固有名、通称、表記揺れを対象にし、役職・関係・説明だけの語は保存前に除外する。保存上は話数付き履歴を保持するが、API へ返す表示用配列では同じ文字列を重複させない。
- 同じ `upToEpisodeIndex` への再生成は全体再実行でよい。真の増分再計算は将来課題とする。

## API

- `GET /api/library/novels/{novelId}/characters?upToEpisodeIndex={episodeIndex}`
  - 指定話時点で見えるキャラクター一覧を返す。
  - 未生成なら `not_generated` と空配列を返す。
- `POST /api/library/novels/{novelId}/character-jobs`
  - キャラクター一覧生成ジョブを投入する。
  - 同一作品に `queued` または `running` の job があれば、新規 job は作らず既存 job を返す。
- `GET /api/library/novels/{novelId}/character-jobs`
  - 作品ごとの生成ジョブ状態を返す。

## 重要度分類

- `importance` は deterministic に計算する。
- v1 では `main`、`regular`、`semi-regular` を使う。
- タグ付けは解釈ぶれと評価基盤不足が大きいため未実装。再開時は許可タグ集合、評価データ、prompt tuning の運用をまとめて設計する。

## テスト観点

- `upToEpisodeIndex` 以下の情報だけが返る。
- 履歴型項目から最新表示文が選ばれる。
- 同一作品への重複ジョブ投入で active job が再利用される。
- LLM 応答の schema mismatch、timeout、retry、部分失敗が扱える。
- 本名・別名・役職名・血縁呼称・偽名の混同は、検証用 fixture 作品「E2E 人物名寄せ検証」で手動または実験スクリプトにより確認する。
- OpenRouter 実呼び出しを伴う確認は常時 CI ではなく、手動または限定 smoke に留める。
