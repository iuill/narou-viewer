---
name: repo-verify
description: Use when changing code in narou-viewer and you need to choose or run the correct validation commands, including package-scoped unit tests, build, verify:fast, verify, and how to report skipped E2E.
---

# Repo Verify

この skill は `narou-viewer` で変更後の検証コマンドを決めるための最短手順です。まず [`AGENTS.md`](../../../AGENTS.md) を読み、必要なら [`docs/testing/testing-strategy.md`](../../../docs/testing/testing-strategy.md) を参照します。

## 基本ルール

- 作業開始時に `git config --get core.hooksPath` を確認する。Dev Container で `.githooks` でなければ post-create の警告と既存設定を確認し、Dev Container 外なら Betterleaks 導入後に `bash scripts/install-git-hooks.sh` を実行する。
- commit / push 前の検査を `--no-verify` で回避しない。CI 相当の全履歴検査が必要なら `bun run security:scan:history` を実行し、検出値を報告へ転記しない。
- Git hook、commit message検査、trusted PR event解決、または機微情報scannerを変更したら`bun run test:security`も実行する。
- コードを変更したら、`bun run lint` の実行を必須とする。
- コードを変更したら、まず `bun run lint` を実行し、その後に高速テスト、最後に build、原則として E2E を検討する。
- `services/novel-fetcher` を変更した場合は、Go 標準ツールチェーンの薄い入口として `bun run verify:novel-fetcher` も実行する。これは `gofmt -l .`、`go test ./...`、`go build -o /tmp/novel-fetcher-check ./cmd/novel-fetcher` の alias であり、Go を Bun workspace package として扱うものではない。
- Go コードを変更した場合は、起動済みの `novel-fetcher` サービスコンテナ内で dev watcher の自動リビルドが失敗していないことをログで確認する。コンテナが未起動、または Docker が使えない場合は、最終報告で理由と代替確認を明記する。
- UI を変更したら、検証コマンドとは別に画面確認フェーズを入れる。既定は [`.agents/skills/ui-review-screenshot/SKILL.md`](../ui-review-screenshot/SKILL.md) の手順で `playwright-cli` コマンドにより `pc-xga` / `ipad-mini` / `iphone-16e` のスクリーンショット確認を行う。
- ドキュメントだけの変更なら、テストは必須ではない。最終報告で未実施であることを明示する。
- E2E を実施しない場合は、理由を最終報告に残す。
- unit test の coverage を確認したいだけなら、`bun run test:unit:coverage` を実行して `coverage/coverage-summary.json` と標準出力の集計を確認する。

## 変更範囲ごとの既定

### viewer-api のみ

```bash
bun run lint
bun run verify:api-go
bun run verify:api-go:contract
```

### Go fetcher sidecar のみ

```bash
bun run lint
bun run verify:novel-fetcher
```

起動済みの `novel-fetcher` サービスコンテナがある場合は、自動リビルド失敗有無もログで確認する。

### フロントエンドのみ

```bash
bun run lint
bun run --filter @narou-viewer/viewer-web test:unit
bun run --filter @narou-viewer/viewer-web build
```

### 両方にまたがる変更

```bash
bun run lint
bun run test:unit
bun run build
```

`services/novel-fetcher` も含む場合は、上記に加えて `bun run verify:novel-fetcher` を実行する。

### 高速確認をまとめて行う場合

```bash
bun run lint
bun run verify:fast
```

### unit coverage を確認する場合

```bash
bun run test:unit:coverage
```

- workspace 全体の Vitest project を対象に coverage を集計する。
- レポートは `coverage/` に出力される。
- 機械的に数値を確認したい場合は `coverage/coverage-summary.json` を見る。
- まず全体値を確認し、その後 package 単位、最後に file 単位で `statements` と `branches` の低い箇所を確認する。

### 最終確認まで行う場合

```bash
bun run lint
bun run verify
```

## E2E の判断

- 実行時挙動、画面遷移、複数サービス連携、キャッシュや service worker 互換性に触れたら `bun run e2e:test:container` を優先する。
- E2E の前に fixture や常駐サービスが怪しい場合は [`.agents/skills/e2e-recovery/SKILL.md`](../e2e-recovery/SKILL.md) を使う。
- read-only smoke だけで足りる場合は [`.agents/skills/e2e-smoke/SKILL.md`](../e2e-smoke/SKILL.md) を使う。
