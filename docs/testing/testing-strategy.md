# テスト方針

## 1. 目的

- Playwright を使う E2E はブラウザ起動、複数サービス起動、fixture 初期化を含むため、回帰確認としては有効だが日常的な変更確認には遅い。
- コード変更時は、まず Playwright を使わない高速なコードレベルテストを通し、その後に Playwright E2E を実行する。
- この文書は、`narou-viewer` に追加する高速テストの方法、採用ツール、優先すべきテスト観点、実行ルールを定義する。

## 2. 基本方針

- テストは次の 2 段階で運用する。
  1. 高速コードレベルテスト
  2. Playwright E2E
- 日常の変更確認、PR 作成前、CI の前段では高速コードレベルテストを優先する。
- Playwright E2E は、ブラウザ実動作や複数サービス連携を確認する最終ゲートとして扱う。
- 高速テストは、できるだけブラウザ起動や Docker 起動を避け、Node 互換 CLI だけで完結させる。
- このリポジトリの検証導線は `bun run ...` を標準入口にするが、`Vitest` / `Vite` / `TypeScript` / `Playwright` 自体は Node エコシステムのツールとして扱う。
- そのため、Bun 移行の目標は「依存解決と workspace orchestration を Bun に寄せる」ことであり、「テストや build から Node 依存を完全に消すこと」ではない。
- `viewer-api-go` と `novel-fetcher` は Bun workspace package ではなく、Go 標準ツールチェーンで検証する。root には `bun run verify:api-go` / `bun run verify:novel-fetcher` という薄い入口だけを置く。

## 3. 採用ツール

### 3.1 テストランナー

- `Vitest` を標準のコードレベルテストランナーとする。
- 採用理由:
  - `viewer-web` が Vite ベースであり、導入負荷が低い。
  - TypeScript / ESM と相性がよい。
  - `viewer-web` と root scripts の TypeScript / ESM テストに使える。
  - watch 実行が軽く、ローカル反復に向く。

### 3.2 フロントエンド補助

- 速度を優先するため、`viewer-web` のコードレベルテストは原則として Node.js 環境で完結する純粋関数を対象にする。
- DOM 依存の挙動、ブラウザ API、実レンダリングを伴う確認は、既定では Playwright E2E に寄せる。
- React コンポーネントや DOM を直接検証するコードレベルテストを追加する場合は、速度への影響が小さいことと、E2E では代替しにくい価値があることを前提条件とする。

### 3.3 Go backend 補助

- `apps/viewer-api-go` は `gofmt`、`go test`、`go build`、API contract test を標準確認にする。
- API ルートのコードレベル統合テストは Go の `httptest` を使用する。
- これにより外部 port を listen せず、Go process 内で request/response を検証できる。
- 外部 HTTP 依存は fake fetcher client / `httptest.Server` で置き換える。
- ファイル永続化はテンポラリディレクトリを使い、`data/` や `data_e2e/` には触れない。
- `viewer-api-go` の internal coverage は CI の `viewer-api-go` job で閾値確認する。

### 3.4 Go sidecar 補助

- `services/novel-fetcher` は `gofmt`、`go test`、`go build` を標準確認にする。
- `bun run verify:novel-fetcher` は monorepo root から呼びやすくするための alias であり、Go の依存管理や package 解決を Bun に寄せるものではない。
- 実サイト取得を伴う検証は既定の高速テストに含めない。HTML parser、rate limiter、storage は fixture / unit test で確認し、外部サイトへの連続アクセスを避ける。

## 4. テストレイヤ

### 4.1 ユニットテスト

- 純粋関数、正規化関数、変換関数、表示ラベル組み立て関数を対象とする。
- 1 ファイル単位、1 関数単位で失敗箇所を特定しやすいことを重視する。
- もっとも高速で、日常的に回す主力テストとする。

### 4.2 コードレベル統合テスト

- モジュール単位で依存を含めて確認するが、Playwright や Docker は使わない。
- 例:
  - `FileStateStore` が実ファイルに対して YAML を読み書きできるか
  - `LibraryService` が fixture ディレクトリから作品一覧や話本文を読み取れるか
  - API route が `httptest` で期待する status / body / header を返すか
- 境界条件、データ契約、永続化、ETag、HTTP ステータスの確認に使う。

### 4.3 Playwright E2E

- UI 操作、複数サービス間連携、レスポンシブ表示、スクリーンショット、ブラウザ依存挙動を確認する。
- 高速コードレベルテストで担保できない部分だけを残し、回帰の主戦場にしすぎない。
- ただし UI 変更時の「画面確認フェーズ」は、通常の E2E 回帰とは別導線で行ってよい。`playwright-cli` による軽いスクリーンショット確認で先に崩れを見つけ、その後に必要な E2E を判断する。

## 5. 優先テスト対象

### 5.1 `apps/viewer-api-go`

#### 5.1.1 `internal/store`

- YAML の正規化:
  - 壊れた値、欠損値、古い値が来ても既定値へ丸められるか
  - `episode_index` が数値/文字列の両方から正規化されるか
  - `position` が 0 以上のみ通るか
  - 旧 `line_number` が来ても `position: 0` へフォールバックするか
  - `scroll.value` が `0..1` に clamp されるか
- 永続化:
  - `Initialize()` で初期 YAML と管理ディレクトリが作られるか
  - `PutReadingState()` で `revision` が増えるか
  - `CreateBookmark()` / `DeleteBookmark()` / `PruneNovelState()` が正しく反映されるか
- 競合:
  - 書き込みが temp file + rename で行われる前提を壊していないか

#### 5.1.2 `internal/library`

- ライブラリ読取:
  - `data/novel-fetcher/library.sqlite` から一覧を構成できるか
  - site + siteWorkID から `novelId` が安定するか
- 話一覧組み立て:
  - `episode_index` と fetcher 側 episode ID の対応が正しいか
  - title / chapter / subchapter の fallback が正しいか
  - `chapter` / `subchapter` / `updatedAt` / `contentEtag` の fallback が正しいか
- 本文変換:
  - novel-fetcher の canonical body から reader document を構築できるか
  - `introduction` / `body` / `postscript` の section 分割が正しいか
  - `plainTextLength` が期待通りか
- ファイル探索:
  - `episodeIndex` に対応する episode JSON が見つかるか
  - 本文未取得では契約通りの status / error を返すか

#### 5.1.3 `internal/fetcher`

- レスポンス正規化:
  - success envelope の `data` を正しく解釈できるか
  - 数値/文字列の混在を吸収できるか
  - task 配列や optional field が欠けていても既定値へ落とせるか
- エラー処理:
  - 非 2xx で fetcher API error を返すか
  - API エラー本文から message を拾えるか
  - 不正な envelope を 502 相当として扱えるか
- 送信内容:
  - download/update/remove/cancel の request body が API 契約通りか

#### 5.1.4 `internal/httpapi` の API ルート

- `httptest` を使い、外部 port なしで検証する。
- 主な観点:
  - 入力不正時の 400
  - 未存在 novel / episode の 404
  - novel-fetcher 未接続時の 502
  - ID 解決不能時の 409
  - `ETag` と `If-None-Match` による 304
  - `/api/library/novels` で既読状態・栞情報がマージされるか

### 5.2 `services/novel-fetcher`

#### 5.2.1 fetcher / rate limiter

- host ごとの待機が独立しているか
- `download.interval` 相当の page interval と、なろう向け step wait が維持されるか
- `context.Context` cancel 中に待機や取得が止まれるか
- 503 / 429 などの backoff 要求を連打しないか

#### 5.2.2 site parser

- 小説家になろうの N コード / URL 正規化
- 目次ページから title、author、story、章、話一覧、改稿日時を抽出できるか
- 本文ページから前書き、本文、後書きを分離できるか
- カクヨム URL 正規化、`__NEXT_DATA__` / Apollo state からの title、author、story、章、話一覧抽出、本文 selector 抽出が fixture で確認されているか

#### 5.2.3 storage

- `data/novel-fetcher/` 配下に従来データと分離して保存されるか
- `library.sqlite` で作品、話、asset の索引を安定して扱えるか
- `episodes/*.json` に canonical body、`raw/episodes/*.html` に取得元 HTML が分離保存されるか
- remote 画像が `assets/episodes/<episodeId>/` へ保存され、canonical body の `img src` が相対パスに書き換わり、`width` / `height` が保持されるか
- 小説本文と画像 CDN が host 単位で別 rate limit bucket になるか
- `viewer-api` が novel-fetcher モードで小説本文ファイルを直接読まず、sidecar API だけを参照するか
- canonical JSON / raw HTML / asset の書き込みが temp file + rename で行われ、保存失敗時に既存ファイルが復元されるか

### 5.3 `viewer-web`

#### 5.3.1 ユーティリティ分離とテスト

- `App.tsx` には表示用・整形用・DOM 依存ロジックが集まっている。
- 純粋関数は `src/` 配下の小さな util module に切り出し、その module をユニットテストする。
- 「巨大コンポーネントを丸ごとテストする」より「表示ロジックを切り出して軽く検証する」を優先する。
- DOM を直接扱う処理は、コードレベルテストに無理に持ち込まず、Playwright E2E で担保する。

#### 5.3.2 優先観点

- ルーティング/URL 解析:
  - `novelId` / `episode` / `pos` の解釈
  - 不正な `pos` や `episode` を無視できるか
- 表示ラベル:
  - 話ラベル生成
  - カクヨム系の friendly label 切り替え
  - 最終既読表示の文言
  - 栞位置の文言
- 栞サマリ:
  - 最新栞の選択
  - 栞更新時の一覧サマリ更新
- 説明文整形:
  - story preview の 120 文字切り詰め
  - 空白正規化
- drag & drop:
  - `text/uri-list` 優先
  - コメント行 `#...` の除外
  - `text/plain` fallback
- 画像ビューア周辺の純粋ロジック:
  - natural size 未取得時の fallback
  - ズーム時の表示幅計算
- Reader 操作:
  - edge zone 判定
  - WebKit 判定

- 一方で、読書 HTML 前処理や画像クリック時の DOM 解釈のようなブラウザ依存挙動は、速度を優先して Playwright E2E の確認対象に残す。

#### 5.3.3 React コンポーネントテストの扱い

- コンポーネントテストは、`App.tsx` を分割して責務が明確になってから追加する。
- ただし高速コードレベルテストの目的は短いフィードバックループなので、既定の `test:unit` に重い DOM 環境を持ち込まない。
- 追加する場合も、対象は限定し、プロジェクト全体の既定環境を DOM 前提にしない。
- 対象候補:
  - 作品一覧
  - 栞一覧
  - novel-fetcher 状態パネル
  - reader settings panel
- ユニットテストとコードレベル統合テストで大半のバグを先に拾う。

## 6. 実行ルール

### 6.1 ローカル開発

- コード変更時は、まず変更箇所に対応する高速コードレベルテストを実行する。
- 高速テストが通ってから build を実行する。
- build が通ってから Playwright E2E を実行する。
- 実行コマンドは `bun run ...` に統一するが、内部で呼ばれる `vitest` / `tsc` / `playwright` は Node 系 CLI として扱う。

推奨順序:

```bash
bun run test:unit
bun run build
bun run verify:novel-fetcher  # services/novel-fetcher を変更した場合
bun run e2e:test:container
```

- 変更範囲が `viewer-api` のみなら、まず API 側の高速テストだけを回してよい。
- 変更範囲が `viewer-web` のみなら、まず web 側の高速テストだけを回してよい。
- ただし PR 前や mainline へ入れる前には workspace 全体の高速テストを回す。

### 6.2 CI

- application CI は独立した job を並列に流し、`viewer-web-build` 完了後に Playwright E2E matrix を開始する。TypeScript coverage 閾値付き unit test、各 Go service の検証、API contract はそれぞれ独立した品質ゲートとして同じ workflow 内で確認する。
- 依存・toolchain 監査は application CI から分離した workflow で PR、main push、manual dispatch、週次実行を扱う。E2E の依存先には追加せず、監査によって application CI の critical path を延ばさない。
- repository-size report は `pull-requests: write` 権限を application CI から分離した PR 専用 workflow で実行する。
- 公開入口の TLS / 認証を含む確認は、配置先や前段 proxy の責務に応じて個別に行う。app repository の常設 CI では、汎用 self-host sample とアプリ本体の検証に留める。
- Playwright E2E job は `viewer-web-build` の成果物を前提に開始し、Go 実行ファイルは同一 workflow run の artifact を service 起動直前に取得する。
- これにより、単純な回帰や入力正規化バグ、YAML 永続化の破壊を短時間で落とせる。
- coverage gate は、TypeScript では `apps/viewer-web/src` を root Vitest coverage の対象にし、Go では `apps/viewer-api-go/internal` と `services/novel-fetcher/internal/...` を各 verify script の対象にする。

## 7. 想定スクリプト構成

- 具体的なスクリプト名は実装時に調整してよいが、考え方は次を基準とする。

```json
{
  "test:unit": "workspace 全体の高速テスト",
  "verify:api-go": "Go viewer-api の gofmt + go test coverage + go build",
  "verify:novel-fetcher": "Go sidecar の gofmt + go test + go build",
  "verify:fast": "test:unit + build",
  "verify": "verify:fast + Playwright E2E"
}
```

- 例:
  - `bun run test:unit`
  - `bun run verify:novel-fetcher`
  - `bun run verify:fast`
  - `bun run verify`

## 8. 導入優先順位

1. `viewer-api-go` の `internal/store` ユニット/統合テスト
2. `viewer-api-go` の `internal/library` ユニット/統合テスト
3. `viewer-api-go` の `internal/fetcher` ユニットテスト
4. `viewer-api-go` の `internal/httpapi` `httptest` テスト
5. `viewer-web` の util 切り出しとユニットテスト
6. 必要になった範囲の React コンポーネントテスト

- 先に backend 側を厚くする理由:
  - 現在の責務上、YAML 読取、永続化、novel-fetcher API 境界、ETag などの不具合は backend 側に集まりやすい。
  - これらは Playwright で再現すると遅いが、コードレベルテストでは高速に検証できる。

## 9. Playwright に残すべき確認

- 画面遷移と主要ユーザーフロー
- 縦書き表示やブラウザ差異
- Service Worker を含む実ブラウザ挙動
- スクリーンショット回帰
- Docker 上の実サービス連携
- DOM 依存の読書 HTML 前処理、画像クリック、ブラウザ API をまたぐ UI 挙動

- 逆に、入力正規化、YAML 読書き、DTO 変換、ラベル組み立て、ETag 判定のようなロジックは可能な限り高速コードレベルテストへ移す。

## 10. 備考

- この文書は、テストを追加・更新するときの判断基準をまとめる。
- 既存アーキテクチャを崩さず、最小の責務分離でテストしやすい形へ整理する。
