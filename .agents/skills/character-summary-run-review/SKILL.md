---
name: character-summary-run-review
description: Use when reviewing saved character-summary experiment runs that are synthetic, self-authored, or explicitly licensed for repository-safe analysis.
---

# Character Summary Run Review

この skill は、保存済みの `character-summary` 実験 run を安全にレビューするための手順です。
既定では外部 API を追加で呼ばず、現在のセッションで比較・分析します。

## 入力データと保存物の制約

- 第三者作品本文、raw HTML、取得済み画像、実作品由来の model output を repository に保存しない。
- review 対象は synthetic fixture、自作データ、または repository で扱えることが明確な利用許諾済みデータに限定する。
- `data/ai-experiments/runs/` は runtime / private analysis data として Git 管理しない。
- review 結果を保存する場合も、実作品由来の固有表現、本文抜粋、raw response を含めない。
- 入力の由来が判断できない run は、内容を引用せず、repository に保存しない private analysis data として扱う。

## 対象

- `data/ai-experiments/runs/<runId>/`
- 主に見るファイル:
  - `manifest.yaml`
  - `prompt-preview.json`
  - `dataset.json`
  - `outputs/*.md`
  - `raw/*.json`

## 読む順序

1. `manifest.yaml` を読んで run 全体の条件、対象モデル、入力データの由来を把握する
2. 入力データが synthetic / 自作 / 許諾済みであることを確認する
3. `prompt-preview.json` と `dataset.json` があれば読み、入力と prompt を押さえる
4. `outputs/*.md` を横並びで比較して、まず差分の当たりを取る
5. 判断が難しい箇所だけ `raw/*.json` を確認する

入力データの由来が不明な場合は、`outputs/*.md` や `raw/*.json` の本文を引用せず、保存物を作らない。

## 見る観点

- 複数モデルで同じ欠点が出ているか
- 単一モデルだけに出ている欠点か
- 問題の主因が prompt か、モデルか、入力整形か
- synthetic fixture に対する事実性が保たれているか
- ハルシネーションが低率か
- 情報量が多すぎないか
- 読みやすく端的か
- batch timing から見て実験用途として重すぎないか

## 既定の出力

ユーザーが保存を望む場合は、既定で `evaluations/agent-run-review.md` を書く。

推奨構成:

- `# Overall`
- `# Input Safety`
- `# Cross-Model Findings`
- `# Prompt-Level Issues`
- `# Model-Specific Issues`
- `# Suggested Prompt Revisions`
- `# Next Run`

## 判断ルール

- 複数モデルで同じ欠点が再現していれば、まず prompt 起因を疑う
- 単一モデルだけなら、モデル固有の癖や性能差として扱う
- 根拠が弱い場合は断定せず、仮説として書く
- 改善案は抽象論より、次 run で差し替えられる prompt 指示として書く
- repository に保存する出力は、本文引用ではなく問題の種類と改善方針を中心に書く

## 追加出力

- 機械可読が必要なときだけ `evaluations/run-review.json` を追加する
- モデル別メモが必要なときだけ `evaluations/model-notes.md` を追加する
- どちらも synthetic / 自作 / 許諾済みデータ由来であることを確認できる場合に限る

## 注意

- 既定では OpenRouter など外部課金 API を増やさない
- 読み切れない大きさの run では、まず `outputs/*.md` を優先し、必要箇所だけ `raw/*.json` を掘る
- ファイルが欠けている場合は、何が見られず、どの判断が弱くなるかを明記する
- repository に残せるか迷う run は保存せず、private/local review として扱う
