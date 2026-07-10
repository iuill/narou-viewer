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
- `--concurrency`: 同時実行数
- `--output-dir`: 保存先。既定は `data/ai-experiments/runs`

モデル直接指定には利用可能な OpenRouter API key が必要。通常は AI 生成設定の共有 OpenRouter key か、key を持つ base profile を使う。

## 保存先

run は `data/ai-experiments/runs/<runId>/` に保存する。`data/` 配下なので、通常は Git 管理しない runtime / analysis data として扱う。実作品由来の比較メモや出力レビューは repository に置かず、必要な場合は synthetic fixture を使った benchmark メモへ置き換える。

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
