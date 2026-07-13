# ドキュメント索引

`docs/` は人間向けの正本、設計判断、運用ドキュメントを置く場所です。エージェント向けの反復手順は [`.agents/skills/`](../.agents/skills) に分離し、この索引からもたどれるようにします。

## 正本

- [`architecture.md`](architecture.md): アーキテクチャ、責務分離、データ境界

## テスト

- [`testing/testing-strategy.md`](testing/testing-strategy.md): 高速コードレベルテストと E2E の役割分担
- [`testing/e2e-setup.md`](testing/e2e-setup.md): E2E セットアップ、Codespaces 差分、smoke、成果物運用

## 開発

- [`development.md`](development.md): Dev Container、toolchain、検証コマンド、E2E fixture の開発手順
- [`ai-experiments.md`](ai-experiments.md): 人物・用語抽出の prompt / model 比較実験と評価手順

## 運用

- [`deployment.md`](deployment.md): 汎用 self-host とデプロイ方針

## ライセンス・報告・貢献

- [`../LICENSE`](../LICENSE): repository のライセンス
- [`../NOTICE.md`](../NOTICE.md): 非提携表示、商標、第三者作品データの扱い
- [`../SECURITY.md`](../SECURITY.md): 脆弱性・secret・第三者作品データ混入の報告方針
- [`../CONTRIBUTING.md`](../CONTRIBUTING.md): issue / PR の注意事項

## 機能別設計

- [`extraction.md`](extraction.md): 人物・用語抽出機能の仕様
- [`publication-info.md`](publication-info.md): 書籍情報とカバー画像表示の仕様
- [`reader-ai-assistant.md`](reader-ai-assistant.md): 本文表示画面の読書AI機能仕様
- [`state-schema-policy.md`](state-schema-policy.md): YAML state schema の互換ポリシー

## エージェント向け Skills

`.agents/skills/` はエージェント向けの discovery root です。repository 共通 skill は通常の tracked file として管理し、下の索引に載せます。個人用の Agent Tools を同じ root に置く場合は `.agents/skills/local-*/` を使います。`local-*` は Git 管理外で、この索引には載せません。agent discovery や lint から見える場合があるため、秘密情報や非公開 endpoint は書かないでください。

- [`.agents/skills/extraction-run-review/SKILL.md`](../.agents/skills/extraction-run-review/SKILL.md): synthetic / 自作 / 許諾済み extraction 実験 run を比較レビューし、prompt 改善案を書く
- [`.agents/skills/repo-verify/SKILL.md`](../.agents/skills/repo-verify/SKILL.md): 変更範囲に応じた検証コマンドの選択
- [`.agents/skills/e2e-recovery/SKILL.md`](../.agents/skills/e2e-recovery/SKILL.md): E2E fixture / service / state の復旧手順
- [`.agents/skills/e2e-smoke/SKILL.md`](../.agents/skills/e2e-smoke/SKILL.md): 内部 E2E service や generic self-host origin の read-only smoke check 判断
- [`.agents/skills/ui-review-screenshot/SKILL.md`](../.agents/skills/ui-review-screenshot/SKILL.md): UI 変更時の `playwright-cli` 画面確認手順
- [`.agents/skills/pr-merge/SKILL.md`](../.agents/skills/pr-merge/SKILL.md): PR の merge と base branch 同期、remote / local 作業 branch の cleanup 手順
