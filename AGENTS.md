# AGENTS.md

## 目的

- このファイルは、`narou-viewer` リポジトリで作業するコーディングエージェント向けのリポジトリ固有ルールを定義する。
- 変更はできるだけ具体的かつ最小限にし、現在のアーキテクチャとデータ境界を崩さないことを優先する。
- `apps/viewer-api-go`、`apps/viewer-web`、`services/novel-fetcher`、共有ランタイムデータに触れる前に一度読むこと。

## リポジトリ概要

- Bun workspaces で管理している monorepo 構成。
- アプリケーション構成:
  - `apps/viewer-api-go`: viewer-api backend
  - `apps/viewer-web`: React + Vite + TypeScript のフロントエンド
  - `services/novel-fetcher`: 取得 sidecar
- 共有ランタイムデータは `data/` 配下に置く。`data/` は runtime / private analysis data であり、第三者作品由来の本文、raw HTML、画像、model output を repository に保存しない。
- アーキテクチャとデータ境界は `docs/architecture.md` に記載されている。
- 機能別仕様は `docs/extraction.md`、`docs/publication-info.md`、`docs/reader-ai-assistant.md`、`docs/state-schema-policy.md` などに記載されている。
- ドキュメント索引は `docs/README.md` を参照する。
- エージェント向けの反復手順は `.agents/skills/` 配下に置き、人間向けの正本を置き換えない。
- `.github/skills` は `.agents/skills` への symlink 生成先として扱い、正本にはしない。
- 個人用の Agent Tools を同じ discovery root に置く場合は `.agents/skills/local-*/` を使う。この namespace は Git 管理外で、共有 docs の Skills 索引には載せない。agent discovery や lint から見える場合があるため、秘密情報や非公開 endpoint を書かない。

## 仕様の優先順位

- 機能仕様の基準: `docs/README.md` から辿れる機能別ドキュメント
- アーキテクチャと責務分離の基準: `docs/architecture.md`
- ローカル起動方法とポートの基準: `README.md`
- 高速テストの詳細方針: `docs/testing/testing-strategy.md`
- 実装とドキュメントが食い違う場合の扱い:
  - バグ修正では、まず現行の実ランタイム挙動を優先して解釈する。
  - 新機能や仕様拡張では、原則として `docs/architecture.md` と機能別ドキュメントを優先する。
  - API 契約や YAML スキーマを、説明なく変更しない。

## 作業方針

- Dev Container では post-create 時に `.githooks/` の `pre-commit` / `commit-msg` / `pre-push` が自動で有効になる。Dev Container 外では Betterleaks を導入後、`bash scripts/install-git-hooks.sh` を一度実行する。
- `pre-commit` は staged diff、`commit-msg` は commit message、`pre-push` は push 対象 commit の diff と message を Betterleaks と repository 固有ルールで検査する。検出を `--no-verify` で回避せず、false positive はルール側で明示的に解消する。
- Betterleaksの`gitleaks:allow` / `betterleaks:allow` markerは信頼せず、すべての検査経路で無効化する。専用GitHub Appが発行する`sensitive-information/commits`をrequired gate、PR metadataをadvisory検査とする。App秘密鍵をPR eventや未信頼データを解析するrunnerへ渡さない。
- 変更前に、影響を受けるファイルを確認してから手を入れる。
- 変更範囲は絞る。依頼に直接関係しない大規模リファクタは避ける。
- 既存の命名、構成、責務分割を尊重する。
- 症状への場当たり対応より、原因に対する修正を優先する。
- ユーザーへの応答、commit message、PR title / body / comment は、特段の指定がない限り日本語で書く。
- PR は、特段の理由や明示的な指定がない限り draft ではなく ready for review で起票する。
- PR を作成・更新する前に `.github/pull_request_template.md` を読み、各セクションを省略せず、該当しない項目にも理由を記載する。追いコミット後は変更内容、ユーザー影響、互換性・移行、検証結果が PR 本文と一致しているか再確認する。
- 仕様、セットアップ、データ契約に影響する変更をした場合は、関連ドキュメントも更新する。
- 破壊的な git 操作は避ける。
- 依頼範囲外のユーザー変更を上書きしない。

## データと秘密情報の安全策

- 第三者作品の本文、長い引用、取得済み raw HTML、取得済み画像、実作品由来の model output を repository に追加しない。
- 再現データ、レビュー用データ、スクリーンショット確認用データは synthetic fixture または利用許諾済みデータを使う。
- API key、cookie、private key、`.env.local`、個人運用環境の実 IP、非公開運用情報を追加しない。
- AI 機能に関する変更では、本文またはその抜粋・要約用テキストが外部 provider に送られる可能性を UI / README / docs で明確に説明する。

## データ境界

- `viewer-api` は `data/` を読み取り、`data/state/*.yaml` を管理する主体である。
- `viewer-api` から `novel-fetcher/works/*` へ直接書き込まないこと。必要な更新は選択中の取得 backend API 経由で扱う。
- `novel-fetcher` の保存データは `data/novel-fetcher/` 配下で管理する。
- オフラインキャッシュ設定などブラウザ固有の状態は、原則としてクライアント側に留める。
- 共有データは複数サービスから参照するが、コンテナ内パスは同一ではない。`viewer-api` 内で `novel-fetcher` コンテナ側の内部パスを決め打ちしない。

## API とスキーマの規律

- `/api/...` 配下の既存エンドポイントはアプリ契約の一部として扱う。レスポンス形式を変える場合は呼び出し側との整合を必ず取る。
- `episodeIndex` は `toc.yaml` の `episode_index` に対応する整数として扱う。
- ETag やキャッシュ制御の挙動は、キャッシュ関連の変更依頼でない限り維持する。
- YAML 永続化処理を変更する場合は、原子的な書き込みと既存データ互換性を保つ。

## 実装ガイド

### バックエンド (`apps/viewer-api-go`)

- 入力検証や正規化は、共通化の明確な利点がない限り route handler 近くに置く。
- `internal/library` や `internal/store` には、小さく閉じた責務の変更を優先する。
- ファイルベースの永続化前提を守る。明示的な依頼なしに DB や外部状態ストアを導入しない。

### 取得 sidecar (`services/novel-fetcher`)

- Go 標準のツールチェーンを使い、Bun workspace package にはしない。
- 検証は `bun run verify:novel-fetcher` を入口にしてよいが、中身は `gofmt`、`go test`、`go build` の薄い alias として扱う。
- Go コードを変更した場合は、`bun run verify:novel-fetcher` に加えて、起動済みの `novel-fetcher` サービスコンテナ内で dev watcher の自動リビルドが失敗していないことをログで確認する。コンテナが未起動で自動リビルドを確認できない場合は、理由と代替確認を最終報告に明記する。
- 小説家になろう / カクヨムの実サイトへアクセスする変更では、User-Agent、host 単位 rate limit、待機、retry、キャンセル可能性を維持する。実装中の確認では外部サイトへの連続アクセスを避け、できる限り fixture や parser unit test で検証する。
- 実 URL を実際に投入して動作検証する場合は、短編または話数の少ない作品を少数だけ使い、同じ URL の連続再試行を避け、失敗原因の確認はまず sidecar ログ・保存済み raw HTML・fixture test で行う。
- `novel-fetcher` は小説家になろう / カクヨムの基本取得に対応する。カクヨムは HTML 内 `__NEXT_DATA__` / Apollo state と本文 selector に依存するため、実サイト構造変更時は fixture ベースの parser unit test を先に更新する。

### フロントエンド (`apps/viewer-web`)

- 現在の単一アプリ構成は、必要性が明確でない限り維持する。
- React + Vite + TypeScript の既存スタイルに合わせる。強い理由なしに状態管理ライブラリやルーティングライブラリを追加しない。
- UI 変更は、デスクトップとモバイルの両方で破綻しない範囲で段階的に行う。
- UI を変更した場合は、機能検証用の E2E とは別に、`playwright-cli` コマンドによる画面確認フェーズを入れる。対象画面は `pc-xga`、`ipad-mini`、`iphone-16e` の 3 パターンを確認し、レイアウト崩れや視認性を見たうえで必要なら再修正する。
- オフライン読書まわりは `public/reader-sw.js` と `src/registerServiceWorker.ts` の流れを前提に互換性を保つ。
- 本文表示画面下部アイコンから開く reader panel を追加・変更する場合は、まず既存の共通要素を再利用できないか確認する。少なくとも `ReaderFloatingPanel`、`reader-overlay-panel--*` 幅指定、`reader-panel-card`、`reader-panel-card--compact`、`reader-panel-card--hero`、`reader-panel-chip`、`reader-panel-link` の利用可否を先に検討し、panel 固有スタイルの新設は共通化で表現できない差分に限定する。
- reader panel 内の見た目を調整する際は、特定 panel だけで完結する装飾を先に足すのではなく、「他の下部アイコン panel でも再利用したい見た目の要素（面・chip・リンク・操作行など）かどうか」を確認してから CSS を追加する。再利用可能なら `styles.css` の reader panel 共通セクションへ寄せる。

### ビルド成果物

- `dist/` 配下の生成物は、ユーザーから明示的に求められない限り手編集しない。
- 変更は `src/` などのソースに対して行い、必要なら生成し直す。

## 検証

- 機微情報検査の CI 相当確認には `bun run security:scan:history` を使う。出力時は Betterleaks の redaction を維持し、検出値をログや報告へ貼らない。外部 validation は有効にしない。
- Git hook、Betterleaks range、禁止 path、公開 IPv4 検査を変更した場合は `bun run test:security` で一時 Git repository を使う回帰テストも実行する。
- コードを変更した場合は、`bun run lint` の実行を必須とする。
- コードを変更した場合は、まず `bun run lint` を実行し、その後に変更箇所に応じた高速コードレベルテストを実行し、最後に build、原則として E2E テストを実施する。
- ドキュメントだけの変更なら、テストは必須ではない。最終報告で未実施であることを明示する。
- E2E テストを実施できなかった場合は、その理由を最終報告で明示する。

### 主な検証コマンド

- workspace 全体の lint / format check: `bun run lint`
- workspace 全体の高速テスト: `bun run test:unit`
- `novel-fetcher` の整形・高速テスト・build: `bun run verify:novel-fetcher`
- 高速テスト + build: `bun run verify:fast`
- 高速テスト + build + E2E: `bun run verify`
- E2E の既定手順:
  - `bun run e2e:fixture:init`
  - `bun run e2e:services:up`
  - `bun run e2e:test:container`
- `data_e2e` の fixture を明示的に作り直す必要がある場合のみ、`bun run e2e:fixture:rebuild` を使う。
- `data_e2e/state` のみ初期化したい場合は、`bun run e2e:state:reset` を使う。
- `data_e2e` の権限や所有者が壊れた場合は、`bun run e2e:repair` を使う。

### 変更範囲ごとの既定

- `viewer-api` のみ:
  - `bun run lint`
  - `bun run verify:api-go`
  - `bun run verify:api-go:contract`
- `novel-fetcher` のみ:
  - `bun run lint`
  - `bun run verify:novel-fetcher`
  - 起動済み `novel-fetcher` サービスコンテナ内の自動リビルド失敗有無をログで確認
- フロントエンドのみ:
  - `bun run lint`
  - `bun run --filter @narou-viewer/viewer-web test:unit`
  - `bun run --filter @narou-viewer/viewer-web build`
- 両方にまたがる変更:
  - `bun run lint`
  - `bun run test:unit`
  - `bun run build`

## エージェント向け補助

- テキスト検索は `rg` を優先する。
- 広いコード探索やシンボル単位の確認では、Serena project server が使える場合は `scripts/serena-query` で LSP ベースの `get_symbols_overview`、`find_symbol`、`find_referencing_symbols` などを活用する。利用できない環境では通常の `rg` とファイル確認で進めてよい。

## 判断に迷う場合

- 次のどれを求められているかを意識する:
  - 最小修正
  - 軽い整理を含む修正
  - ドキュメントや仕様更新まで含む修正
- 安全な仮定で進められるなら進め、その仮定は最終報告で明示する。
