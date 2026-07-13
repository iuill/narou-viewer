# AI 実験・評価手順

この文書は、人物・用語抽出の prompt / model 比較実験を保存し、後からレビューするための開発用評価・チューニング手順である。現行の CLI と review skill の入口をまとめる。

repository に保存する実験入力、出力、review は synthetic fixture、自作データ、または利用許諾済みデータに限定する。第三者作品由来の本文、抜粋、要約、model output は repository に保存しない。

外部 LLM provider を使う実験では、対象本文またはその抜粋・要約用テキストが設定した provider に送信される場合がある。実験実行者は、利用するデータの権利、provider の規約、保存先の扱いを確認してから実行する。

関連:

- [`scripts/run-extraction-experiment.mjs`](../scripts/run-extraction-experiment.mjs)
- [`.agents/skills/extraction-run-review/SKILL.md`](../.agents/skills/extraction-run-review/SKILL.md)

## 目的

- synthetic fixture、自作データ、または利用許諾済みデータについて、同じ話数範囲、同じ prompt 条件で複数モデルを比較する。
- prompt preview、raw response、整形済み出力、実行条件を run 単位で保存する。
- 保存済み run を横断レビューし、prompt 起因の問題と次の改善案を残す。

## 実行

入口は root script の `experiment:extraction:run`。

```bash
bun run experiment:extraction:run -- \
  --novel-id <synthetic-or-licensed-novel-id> \
  --up-to-episode-index 18 \
  --model openai/gpt-4.1-mini \
  --model anthropic/claude-haiku-4.5 \
  --concurrency 3
```

主なオプション:

- `--novel-id`: 対象データの `novelId`。repository に保存する review では synthetic fixture、自作データ、または利用許諾済みデータに限定する
- `--up-to-episode-index`: 人物・用語抽出の上限話
- `--profile`: 保存済み AI 生成 profile を使う
- `--profiles-file`: profile 定義をファイルから読む
- `--model`: 比較対象モデル。複数指定できる
- `--system-prompt-file`: system prompt を一時差し替えする
- `--reasoning-effort`: OpenRouter の `reasoning.effort` を実験中だけ指定する。`none` / `minimal` / `low` / `medium` / `high` / `xhigh` / `max` を受け付け、未指定時は provider の既定値を使う。指定時は未対応 provider が値を無視しないよう `require_parameters=true` を強制し、`--require-parameters false` との併用は拒否する
- `--concurrency`: 同時実行数
- `--output-dir`: 保存先。既定は `data/ai-experiments/runs`

モデル直接指定には利用可能な OpenRouter API key が必要。通常は AI 生成設定の共有 OpenRouter key か、key を持つ base profile を使う。

読書AIを含む隔離した `viewer-api` プロセス全体で reasoning effort を比較するときは、`OPENROUTER_REASONING_EFFORT=xhigh` のように環境変数を指定できる。実験 runner の `--reasoning-effort` が指定されている抽出リクエストでは、リクエスト側の値を優先する。未指定時の通常動作は provider の既定値のままであり、この環境変数は本番の既定設定を変更せずに行う一時的な比較用途に限定する。不正な値を設定した場合は、原因を起動ログに残して `viewer-api` の起動を中止する。APIは解決した要求値を `reasoning.requestedEffort`、由来を `reasoning.source`、provider絞り込みを `reasoning.requireParameters` として返す。runnerと読書AI usage snapshotはこのサーバー報告値を保存し、providerが実際に採用した値を意味する `effectiveEffort` とは呼ばない。

## 保存先

run は `data/ai-experiments/runs/<runId>/` に保存する。`data/` 配下なので、通常は Git 管理しない runtime / analysis data として扱う。実作品由来の比較メモや出力レビューは repository に置かず、必要な場合は synthetic fixture を使った benchmark メモへ置き換える。

複数 run を比較するときは、同じ入力・話数範囲・prompt 条件で原則3回以上実行し、人物の完全一致、用語の recall / precision、処理時間、入出力 token、概算費用を集計する。読書AIも比較する場合は、根拠と推測の分離に加えて、現在話より後の情報を漏らさないことを確認する。横断的な評価レポートをローカルへ残す場合は `data/ai-experiments/reports/` に保存し、run と同様に Git 管理しない。

保存物の考え方:

- 実験条件と manifest
- 対象 dataset
- prompt preview
- model ごとの raw response
- model ごとの整形済み出力
- review 結果

## レビュー

保存済み run の比較には `extraction-run-review` skill を使う。

レビューで見る観点:

- 事実性
- ハルシネーションの少なさ
- 情報量が過剰でないこと
- 読みやすさ
- 複数モデルに共通する prompt 起因の問題
- 次の prompt 改善案

モデルごとの優劣だけでなく、複数モデルに共通する欠点を優先して見る。共通して崩れる場合は、モデル選定より prompt / 入力整形の改善対象として扱う。

## 運用ルール

- 1 run では比較軸をできるだけ 1 つに絞る。
- fallback は原則無効にし、実モデル差を見やすくする。
- 実験対象モデルは最初は 3 から 5 個に抑える。
- 同じ入力で繰り返す run には prompt version や prompt file を明示する。
- 良かった出力だけでなく、なぜ良かったか、何が再現しているかを review に残す。
