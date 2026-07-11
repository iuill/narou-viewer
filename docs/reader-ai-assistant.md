# 読書AI機能

この文書は、本文表示画面の「読書AI」に関する機能仕様をまとめる。全体の責務分離、データ境界、API 一覧は [`architecture.md`](architecture.md) を優先する。

## 目的

- 長編小説を読み直すときに、人物、用語、状況、直近の出来事を思い出しやすくする。
- 作品内検索、本文ロード、生成済みの人物・用語一覧参照を AI が tool として使い、根拠を集めて回答する。
- 未読話の情報を混ぜないことを最優先する。

## 導線

- 本文表示画面の FAB「読書AI」から reader panel として開く。
- 目次、栞、表示設定、キャラクター一覧と同じ reader panel 系 UI に揃える。
- チャット履歴は、同一作品を開いている間はクライアント側で保持し、会話履歴そのものをサーバ側の正本 state としては永続化しない。
- ただし、読書AIの usage / run 分析用 snapshot は `state/ai_usage.sqlite` に保存し、message 長、answer 長、tool request / result のサニタイズ済み概要を後追い確認できるようにする。

## ネタバレ境界

- 境界は `episodeIndex` を基準にする。
- 本文画面では「現在話を含む」を既定で無効にし、直前話を上限とする。有効にした場合だけ現在表示中の話を上限へ含める。第1話では、有効にするまで質問を送信できない。
- `viewer-web` は `novelId`、選択した上限話、直近会話履歴、ユーザー発話を送る。同一作品では話移動や境界切替後も会話を保持し、各発話にその時点の参照上限を記録する。過去の回答は読者が既に得た情報として履歴に含める一方、新しい検索、本文ロード、人物・用語情報参照にはリクエスト時点の上限を適用する。
- `viewer-api` は目次と照合して境界を決め、tool 実行時にも `maxEpisodeIndex` を強制する。
- 境界外の検索、本文ロード、人物・用語情報参照はサーバ側で拒否する。

## 実行境界

- 読書AIの agent loop は `viewer-api` 内で動く。
- LLM 接続は internal AI module の OpenRouter chat/tool calling 経路を使う。
- 作品本文や state へのアクセスは `viewer-api` の local tool に閉じ込める。AI 実行層が `.narou/*`、`小説データ/*`、`novel-fetcher/works/*` を直接読む構成にはしない。
- 将来 MCP endpoint を追加する場合も、ネタバレ境界の最終検証は `viewer-api` 側に残す。

## 主要 tool

- `get_current_episode`: 現在話のタイトルと本文抜粋を返す。
- `get_previous_episode`: 前話のタイトルと本文抜粋を返す。
- `load_episode`: 指定話を境界内で読み込む。
- `load_episode_range`: 最大20話の範囲を読み込む。広い振り返りでは `output: "summary"` と `summaryPurpose` / `summaryFocus` を使い、中間要約を返す。
- `search_episodes`: 最大50話の範囲で作品内検索し、話数、話タイトル、スニペット、本文位置を返す。
- `search_full_text`: 現在のネタバレ境界内を広く検索し、本文全体ではなく run 内限定の `hitId`、話数、話タイトル、スニペット、本文位置、score を返す。初出、過去の言及、長編全体の具体語探索に使う。検索結果は score 上位の `topMatches` と、既読範囲を話数帯で横断する `coverageMatches` に分け、`matches` には両者を統合して返す。`maxResults` は `topMatches` の最大数で、`matches` は `coverageMatches` 追加により `maxResults` を超える場合がある。候補総数、マッチした話数、最初/最後のマッチ話数、score 上位枠の打ち切り有無、未返却候補数は metadata として返す。query は短い具体語検索に限定し、長すぎる query や term 過多の query は回復可能な tool error とする。
- `load_passages`: 同一 reader-assistant run 内で `search_full_text` が返した `hitId` の周辺本文だけを読み込む。未知 `hitId` や境界外 context は回復可能な tool error とし、再検索を促す。
- `get_character_snapshot`: 生成済みキャラクター一覧から、指定話時点で見える情報だけを返す。生成済み frontier が境界より手前なら `partial` と生成済み範囲の人物を返し、本文検索への fallback を案内する。
- `get_term_snapshot`: 生成済み用語一覧から、指定話時点で見える用語名、読み、カテゴリ、説明を最大30件返す。人物側の committed frontier を共有し、用語profileだけが先行した未コミット分は返さない。frontier が境界より手前なら人物と同様に `partial` を返す。

20話または50話を超える範囲指定は agent loop 内で回復可能な tool output として返し、モデルに分割再実行を促す。境界違反や危険な入力は安全上のエラーとして扱う。

## API

- `POST /api/library/novels/{novelId}/reader-assistant/chat`
  - 非 streaming の最終回答を返す。
- `POST /api/library/novels/{novelId}/reader-assistant/chat/stream`
  - `application/x-ndjson` で `status`、`tool_call`、`tool_result`、`result`、`error` を返す。
- 読書AIの run / request 単位の usage は AI機能ワークスペースの「読書AI利用統計」で確認できる。

## テスト観点

- 境界外の `load_episode` / `search_episodes` / `get_character_snapshot` / `get_term_snapshot` が拒否される。
- `load_episode_range` と `search_episodes` の範囲上限が守られる。
- mock LLM で tool call を伴う streaming 応答が成立する。
- UI が設定未完了、検索中、本文ロード中、長い回答、エラーを表示できる。
