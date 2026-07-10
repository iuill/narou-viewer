# 人物・用語抽出機能

本文から人物一覧と作品固有の用語一覧を同じ抽出 response で生成する機能の仕様を定める。全体の責務分離は [`architecture.md`](architecture.md) を優先する。

## 境界と表示

- `episodeIndex` をネタバレ境界とし、人物・用語とも指定話以下の履歴だけを表示する。
- 本文画面では「現在話を含む」を既定で無効にし、直前話を生成・表示境界とする。有効にした場合だけ現在表示中の話を境界へ含める。第1話では、有効にするまで生成を実行できない。
- `GET /api/library/novels/{novelId}/characters` は人物一覧、`GET .../terms` は用語一覧を返す。
- 両 API は character profile の committed frontier を共有する。term profile が先行しても、人物保存済み話より未来の用語は公開しない。
- 用語の `reading`、`category`、`description` は履歴として保持し、境界以下の最新 snapshot を選ぶ。読みは本文に明示された場合だけ保存する。
- 生成済みで用語が0件の場合も `terms: []` の document を保存し、API は `ready` を返す。
- 要求境界より committed frontier が手前の場合、人物・用語 API は `partial` と生成済み範囲の一覧を返す。`not_generated` は生成済み frontier 自体がない場合に限る。

## pipeline と保存

- serial、parallel identity、discovery + parallel correction の3戦略が、detail extraction 1 response から人物と用語を同時生成する。
- response の `terms` は必須で、欠落または `null` は job failure とする。
- 保存順は term profile、character events/profile の順。character frontier を commit marker とし、両方の保存後だけ checkpoint を削除する。
- retry / reprocess は置換境界以降の人物・用語履歴を削除してから再適用する。term だけ先行した partial write も character frontier で隠し、retry で収束させる。
- 旧 character-only state は増分生成しない。`DELETE .../extraction` でクリアして再生成する必要がある。

保存先:

- `state/character_profiles/*.yaml`
- `state/character_events/*.yaml`
- `state/term_profiles/*.yaml`
- `state/extraction_jobs/*.yaml`
- `state/extraction_jobs/index/*.yaml`
- `state/extraction_jobs/checkpoints/*.json`

## API

- `GET /api/library/novels/{novelId}/characters?upToEpisodeIndex=...`
- `GET /api/library/novels/{novelId}/terms?upToEpisodeIndex=...`
- `GET/POST /api/library/novels/{novelId}/extraction-jobs`
- `DELETE /api/library/novels/{novelId}/extraction`
- `/api/ai-generation/playground/extraction` と stream final event は `characters` と `terms` を返す。

## 移行互換

- settings は `extraction_strategy_models` を保存し、旧 `character_summary_strategy_models` は read fallback のみ行う。
- 環境変数は `EXTRACTION_*` / `VIEWER_EXTRACTION_TIMING_LOG` を優先し、旧 `CHARACTER_SUMMARY_*` / `VIEWER_CHARACTER_SUMMARY_TIMING_LOG` は read fallback として残す。
- 起動時に旧 `state/character_jobs` を `state/extraction_jobs` へ best-effort rename し、clear/reset は新旧両方を削除する。
- 旧 usage row の feature 名は表示互換のため読み取れる。

## follow-up

読書 AI 用の `get_term_snapshot` tool は本変更には含めず、別タスクで追加する。
