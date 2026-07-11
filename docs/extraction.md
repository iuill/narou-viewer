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

- serial、parallel identity、discovery + parallel correction の3戦略が、detail extraction 1 response から人物と用語を同時生成する。抽出方式は人物・用語を含む抽出ジョブ全体へ適用する。
- parallel identity と discovery + parallel correction の detail extraction は、各本文バッチを人物用・用語用に二重送信せず、1回の並列リクエストで人物差分と用語の事実差分を同時抽出する。
- 並列方式の同時LLMリクエスト数は「AI生成 > 設定 > 人物・用語抽出」で1〜20に設定し、既定値は3とする。discoveryとdetail extractionの両方へ同じ上限を適用し、serialには適用しない。
- `characterUpdates` は現在バッチの差分だけを受け取り、既存人物の初登場話は更新 response で変更しない。
- discovery の人物名候補は response の話数を当該バッチで検証し、最終補正では既存の名前・別名の話数を維持する。補正理由は物語上の人物履歴へ保存しない。
- 人物の同一性判明は `identity_merge_events` に source / target ID と有効話数を保存する。明示的な `mergeProposals` は返却した runtime batch の境界、identity resolver の判定は生成上限を有効話数とし、それより前の表示境界では別人物のまま投影する。
- 並列バッチの用語説明は、そのバッチで新しく判明した事実差分として受け取り、`description_facts` に話単位で保存する。表示時だけ境界以下の事実を合成し、中間話ごとの累積 snapshot を重複保存しない。後続プロンプトへ渡す説明は長さを制限する。
- term profile は `description_facts` 追加後も `schema_version: 1` を維持する。新ビルドが保存した profile を旧ビルドへロールバックして読み込むと、旧ビルドは未知fieldを無視して事実差分を表示できないため、ロールバック前にstateを退避し、再度新ビルドへ戻した後に再生成する。
- character event / profile の `identity_merge_events` も旧ビルドでは無視される。旧ビルドで保存し直すと時系列identity情報を失うため、同じくロールバック前にstateを退避する。
- snapshotを持たない用語は、表示境界以下の事実をすべて連結して説明を構築する。長編で説明が長くなる場合の表示要約・折りたたみはfollow-upとする。
- serial は従来どおり直前までの用語 snapshot を次バッチへ渡し、LLM response 自体に自己完結型 snapshot を返させる。
- response の `terms` は必須で、欠落または `null` は job failure とする。
- 人物・用語の履歴や名前事前発見の話数が不正、または現在の runtime batch 外の場合は、項目を黙って捨てず job failure とする。structured output を保証しない provider でも、誤った話数を保存してネタバレ境界を壊さないことを優先する。
- provider が応答した JSON のdecode、正規化、話数境界検証に失敗した場合や、空応答・`finish_reason` による切断の場合は、同じpromptで1回だけ再生成する。再生成後も契約不正ならjobを失敗させ、両attemptのtoken usageを記録する。通信・rate limit等のretryはprovider共通層で別に扱う。
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
- 環境変数は `EXTRACTION_*` / `VIEWER_EXTRACTION_TIMING_LOG` のみを使用する。旧 `CHARACTER_SUMMARY_*` / `VIEWER_CHARACTER_SUMMARY_TIMING_LOG` を使用している `.env.local` は利用者側で更新する。
- 起動時に旧 `state/character_jobs` を `state/extraction_jobs` へ best-effortで移行する。新旧ディレクトリが併存する場合もファイル単位で移し、同名で内容が異なる旧ファイルは `state/extraction_jobs/legacy_conflicts` へ退避する。clear/reset は新旧両方を削除する。
- 旧 usage row の feature 名は表示互換のため読み取れる。
- settings / job state の旧名称互換は移行猶予後に [#2](https://github.com/iuill/narou-viewer/issues/2) で削除する。
