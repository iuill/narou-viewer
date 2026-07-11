# 開発ガイド

このドキュメントは、narou-viewer を開発・検証するための手順をまとめます。通常の self-host 起動だけなら、先に [`README.md`](../README.md) のクイックスタートを参照してください。

## Dev Container セットアップ

1. このリポジトリを VS Code で開きます。
   - 複数の git worktree を別 Dev Container として同時起動する場合は、各 worktree フォルダ自体を VS Code で開きます。親フォルダを VS Code workspace として開く運用とは混ぜないでください。
   - 2 つ目以降の worktree では `.devcontainer/.env.example` を `.devcontainer/.env` にコピーし、ホスト側公開ポートを重複しない値へ変更してください。通常の単一 Dev Container では `.devcontainer/.env` は不要です。
2. `Dev Containers: Reopen in Container` を実行します。
   - `viewer-dev` には Dev Containers の `github-cli` feature 経由で `GitHub CLI` (`gh`) もインストールされます。
   - `viewer-dev` は `mcr.microsoft.com/devcontainers/typescript-node:1-22-bookworm` を薄く拡張したイメージを使い、`ja_JP.UTF-8` ロケールを生成して `LANG` / `LC_ALL` に設定し、タイムゾーンも `Asia/Tokyo` に揃えます。Python は `python3` に加えて `python` でも呼べるようにしてあり、Go 1.25.12 (`GOTOOLCHAIN=local`) と SQLite CLI (`sqlite3`) もインストールします。
   - `postCreateCommand` では workspace 依存の `bun install` に加えて、グローバル CLI として `@openai/codex` (`codex`) と `@github/copilot` (`copilot`) も `bun add -g` で固定版インストールされます。coding agent 向けのブラウザ操作 CLI として `@playwright/cli` (`playwright-cli`) も固定版インストールされ、Go LSP の `gopls` も導入されます。
   - コンテナ起動時に `.github/skills` が `.agents/skills` へのシンボリックリンクとして自動作成され、`GitHub Copilot CLI` など `.github/skills` を参照する環境から project skills として見える形になります。
3. コンテナ起動後、次を実行します。

```bash
bun run dev
```

Dev Container image には固定版の Betterleaks が含まれ、`postCreateCommand` は他の開発ツールと同じく版を確認して、不足または不一致なら checksum 検証付きで再導入します。同時に、この clone の `core.hooksPath` を `.githooks` に設定します。既存の `core.hooksPath` が別の値なら上書きせず、hook 有効化を見送って警告します。`pre-commit` は staged diff、`pre-push` は push 対象 commit を検査し、同じ検査を GitHub Actions でも実行します。Betterleaks の外部 validation は有効にせず、検査中に候補 credential を外部 API へ送信しません。

PR の機微情報検査は `pull_request_target` で default branch 版 scanner だけを実行し、PR head は checkout せず commit object として検査します。Betterleaks の GitHub sourceでPR本文とコメントも検査しますが、read-only tokenだけを渡し、外部 validationは有効にしません。repository ruleset では `trusted sensitive information` check を required に設定してください。通常の `pull_request` CI にも同じ検査を残し、導入直後や設定不備を含む defense in depth として扱います。

必要なら先に `.env.sample` を `.env.local` へコピーして値を調整してください。root の `.env.local` が存在する場合、`bun run dev` と各 app の主要 script から自動で読み込みます。シェルや CI で明示した環境変数は `.env.local` より優先されます。

`bun: command not found` になった場合は、`postCreateCommand` の反映前か、ターミナルの `PATH` に `~/.bun/bin` が入っていない可能性があります。次を一度実行してからターミナルを開き直してください。

```bash
bash .devcontainer/scripts/install-bun-and-deps.sh
```

このスクリプトは `bubblewrap` / `ripgrep` の補完、Bun 本体、workspace 依存、`Codex CLI` / `Copilot CLI` / `@playwright/cli` (`playwright-cli`) の固定版グローバル CLI、`gopls` をまとめて導入します。既定では同じ版がすでに入っていれば再インストールを省略します。`Codex could not find system bubblewrap at /usr/bin/bwrap` という警告が出た場合も、同じスクリプトで `bubblewrap` をインストールできます。インストール後にターミナルや Codex セッションを開き直すと警告が消える想定です。

4. ブラウザで `http://localhost:5173` を開きます。`.devcontainer/.env` で `VIEWER_WEB_HOST_PORT` を変更した worktree では、そのポートを使います。
5. API の疎通確認は `http://localhost:8080/api/health` で行えます。`.devcontainer/.env` で `VIEWER_API_HOST_PORT` を変更した worktree では、そのポートを使います。
6. `bun run dev` では `viewer-web` と `viewer-api` を起動します。
7. 小説のダウンロード・更新・削除は viewer UI から行います。`novel-fetcher` sidecar は Dev Container でも外部 publish しません。

PWA のインストール確認を行う場合は、`bun run --filter @narou-viewer/viewer-web build` 後に `bun run --filter @narou-viewer/viewer-web preview` を使い、ブラウザで到達できる preview URL で installability を確認してください。PC なら `http://localhost:4173`、iPhone / iPad 実機なら同じネットワークから到達できる `http://<viewer-dev の IP>:4173` を Safari で開き、共有メニューから「ホーム画面に追加」を使います。

## git worktree を Dev Container で並行利用する場合

Windows ホスト側で `git worktree add` すると、worktree の `.git` 参照や Git 内部メタデータに Windows パスが入り、Dev Container 内で解決できない場合があります。worktree は Dev Container 内で作成してください。

既存 clone を並行開発用の親フォルダ配下へ移す場合の例:

```text
C:\path\to\repos\narou-viewer\
  narou-viewer-main\
  narou-viewer-feature-x\
```

`narou-viewer-main` を VS Code で開いて Dev Container に入った後、コンテナ内で作業ブランチ用 worktree を作成します。

```bash
cd /workspaces/narou-viewer-main
git fetch origin
git worktree add ../narou-viewer-feature-x -b feature-x origin/main
```

Dev Container 内では、現在の worktree が `/workspaces/${localWorkspaceFolderBasename}` として開かれます。`viewer-dev` には現在の worktree が従来どおり `/workspace` にも bind mount され、親フォルダは `/workspaces` に bind mount されます。

作成した worktree は Windows 側からも `C:\path\to\repos\narou-viewer\narou-viewer-feature-x` として見える想定です。そのフォルダを別 VS Code window で開き、Dev Container で reopen してください。

## 開発運用ルール

- Dev Container では post-create 時に機微情報検査用の Git hooks が自動で有効になります。Dev Container 外では Betterleaks を導入後、`bash scripts/install-git-hooks.sh` を一度実行してください。
- コミットメッセージは日本語で記述してください。
- Pull Request のタイトルと本文は日本語で記述してください。
- Pull Request へ追いコミットする場合は、PR 本文の更新要否も確認し、必要であれば更新してください。

## ランタイムメモ

- 開発時は `viewer-web` と `viewer-api` を `viewer-dev` コンテナ内のプロセスとして動かし、`novel-fetcher` と E2E 用常駐サービスは sidecar コンテナとして動かします。
- `bun run dev` は `VIEWER_API_DEV_CORS=1` を付けて `viewer-api` を起動し、LAN / モバイル端末から Vite dev server へアクセスする開発フローを許可します。本番ではこの fallback を有効にせず、同一 Host または `VIEWER_API_ALLOWED_ORIGINS` の明示 allowlist だけを CORS 許可にします。
- 取得 sidecar は `novel-fetcher` です。作品一覧・目次・本文は sidecar の内部 API 経由で読み、保存済み asset 配信時だけ `VIEWER_DATA_DIR/novel-fetcher` 配下の共有ファイルを検証して返します。`novel-fetcher` は小説家になろうとカクヨムの基本取得に対応します。
- `novel-fetcher` への操作は `viewer-web` -> `viewer-api` -> `/api/fetcher/*` を正規経路とし、sidecar API には compose 外部から直接アクセスしません。旧 `/api/narou/*` 互換 API は廃止済みです。
- `.agents/skills` は Dev Container / Codespaces のコンテナ起動時に `.github/skills` として symlink 連携されるため、`GitHub Copilot CLI` など `.github/skills` を参照する環境から同じ skill 群を project skills として再利用できます。
- 共有データは同じホストディレクトリ `data/` を見ますが、コンテナ内パスは異なります。
  - `viewer-api`: `/workspace/data`
  - `novel-fetcher`: `/data/novel-fetcher`
- Dev Container 内から sidecar を手動で `docker compose` 再作成する場合、`HOST_DATA_DIR=/workspace/data` のようなコンテナ内パスを渡さないでください。Docker daemon から見たホスト側 `data/` パスを渡す必要があります。マウント元の確認は `docker ps --format '{{.Names}}'` で対象 container 名を確認し、`docker inspect <container-name> --format '{{.Name}} {{range .Mounts}}{{.Source}}=>{{.Destination}} {{end}}'` で行えます。
- オフライン読書用の話本文キャッシュと容量設定はサーバではなくブラウザローカルに保持します。

## Bun / Node ツールチェーン方針

- このリポジトリでは、依存解決と workspace の入口を `bun install` / `bun run` に統一します。
- 日常の依存同期は lockfile を変更しない `bun run install:locked` を標準とし、`bunfig.toml` では新規公開から 21 日未満の版を既定で避けます。依存の追加や意図的な更新だけを `bun add` / `bun update` で行ってください。
- 一方で、`tsc` / `vite` / `vitest` / `playwright` は引き続き Node エコシステムのツールとして扱います。Bun 管理の workspace から呼び出しますが、「Node 完全排除」は現時点の目標にしません。
- そのため、日常運用では「Bun を標準導線にする」「Node 依存ツールは Bun から起動する」を両立させます。
- 新しい script を追加するときは、まず `bun run ...` を入口にし、Node 専用 CLI を無理に `--bun` へ寄せないでください。
- Dev Container は `mcr.microsoft.com/devcontainers/typescript-node:1-22-bookworm` ベースの `viewer-dev` イメージを使っており、Bun と Node の両方が使える前提です。`viewer-dev` では `ja_JP.UTF-8` ロケール、`Asia/Tokyo` タイムゾーン、Go 1.25.12 (`GOTOOLCHAIN=local`) を有効化しています。CI も同じ考え方で運用します。
- CI では `bun run audit:dependencies` で high severity の依存脆弱性も常時監査します。
- 依存差分のレビューは GitHub Actions の `Dependency Review` workflow を使い、`pull_request` のみで実行します。これは push ごとの再検査ではなく、「その PR が新たに持ち込む依存変更」を確認するためです。
- 既知の悪性版や脆弱性通知は Dependabot alerts と CI の dependency audit で補完します。
- Dependabot の version updates は、このリポジトリの `minimumReleaseAge` と運用ノイズの兼ね合いを見て、既定では前提にしません。必要になったときだけ個別に導入を検討します。

## Go ツールチェーン方針

- Go patch version の正本は root の [`.go-version`](../.go-version) です。`go.mod`、Docker image tag、Dev Container / E2E compose、CI の `setup-go` がこの値からずれていないことと、Dev Container feature から Go を二重導入していないことを `bun run audit:go:toolchain` で検査します。`bun run audit:go` でも同じ検査を先に実行するため、CI ではズレた時点で audit job が失敗します。
- Go は `services/novel-fetcher` の取得 sidecar 用に使います。Bun workspace の package としては扱わず、Go の検証は Go の標準コマンドで行います。
- Dev Container / sidecar / E2E サービスは named volume `narou-viewer-go-cache` を共有し、`GOCACHE=/go/.cache/go-build-shared`、`GOMODCACHE=/go/pkg/mod-shared` を使います。起動時に短い init service が `/go` 配下を `E2E_SERVICE_USER`、未指定時は `1000:1000` に合わせて初期化します。
- ただし monorepo ルートからの入口として、薄い alias `bun run verify:novel-fetcher` を用意しています。これは `services/novel-fetcher` へ移動して `gofmt -l .`、`go test ./...`、`go build -o /tmp/novel-fetcher-check ./cmd/novel-fetcher` を実行するだけです。
- `bun run verify:fast` は従来どおり Bun / TypeScript workspace の高速確認です。`novel-fetcher` を変更した場合は、別途 `bun run verify:novel-fetcher` も実行してください。
- 小説家になろう / カクヨムの実 URL を投入して動作検証する場合は、アクセス過多によるアクセス制限を避けるため、短編または話数の少ない作品を少数だけ使い、同じ URL の連続再試行を避けてください。失敗原因の調査は、まず sidecar ログ、保存済み raw HTML、fixture ベースの parser unit test で行います。

## 高速テスト

Playwright E2E の前段として、Vitest による高速なコードレベルテストを追加しています。日常的な変更確認では、まずこちらを回してください。
速度を優先するため、`viewer-web` のコードレベルテストは純粋ロジック中心に留め、DOM 依存の挙動やブラウザ実動作は Playwright E2E で担保します。

```bash
bun run test:unit
```

build まで含めて確認する場合:

```bash
bun run verify:fast
```

lint / format を CLI で確認する場合:

```bash
bun run lint
bun run format:check
```

必要なら整形は次でまとめて反映できます。

```bash
bun run format:write
```

E2E まで含めた最終確認:

```bash
bun run verify
```

`novel-fetcher` を変更した場合:

```bash
bun run verify:novel-fetcher
```

変更範囲がフロントエンドだけなら、workspace 単位ではなく package 単位でも実行できます。

```bash
bun run --filter @narou-viewer/viewer-web test:unit
```

## E2E セットアップ

- 既定の実行経路は `bun run e2e:test:container` です。
- E2E の `viewer-api-e2e` は `viewer-api` を既定で起動します。
- fixture 初期化、Codespaces 差分、`DOCKER_API_VERSION` 回避策、smoke、成果物運用は [`testing/e2e-setup.md`](testing/e2e-setup.md) にまとめています。
- 復旧や smoke の判断をエージェントへ委ねる場合は [`.agents/skills/e2e-recovery/SKILL.md`](../.agents/skills/e2e-recovery/SKILL.md) と [`.agents/skills/e2e-smoke/SKILL.md`](../.agents/skills/e2e-smoke/SKILL.md) を参照してください。

## 開発構成図

現在の Dev Container 構成は [`.devcontainer/docker-compose.yml`](../.devcontainer/docker-compose.yml) と [`architecture.md`](architecture.md) を正本として確認します。

## リポジトリ規模の確認

公開 repository では、commit 履歴や coverage 推移をまとめた静的統計ページは生成しません。リポジトリ規模や言語別 footprint は、Pull Request ごとに CI の `Repository size report` で確認します。

将来 public repository 上で長期的な coverage 推移や履歴ダッシュボードが必要になった場合は、public repository の初回 commit 以降を対象にした新しい記録方式として設計します。
