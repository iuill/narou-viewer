# E2E セットアップと運用

この文書は `narou-viewer` の E2E 実行経路、環境差分、復旧手順をまとめる。高速テスト全体の方針は [`testing-strategy.md`](testing-strategy.md)、エージェント向けの実行判断は [`.agents/skills/e2e-recovery/SKILL.md`](../../.agents/skills/e2e-recovery/SKILL.md) と [`.agents/skills/e2e-smoke/SKILL.md`](../../.agents/skills/e2e-smoke/SKILL.md) を参照する。

## 1. E2E サービス構成

`viewer-dev` 起動時には、コード変更後すぐ検証できるように E2E 用常駐サービスもあわせて起動する。

- `novel-fetcher-e2e`: テスト用 `novel-fetcher`
- `viewer-api-e2e`: `data_e2e` を読む API
- `viewer-web-e2e`: E2E 向け web
- `playwright-e2e`: 公開 GHCR に置いた軽量 Playwright image をベースにするテストスイート実行用サービス

`viewer-api-e2e` / `viewer-web-e2e` は `viewer-dev` の起動直後からコンテナとして常駐するが、workspace の Bun 依存関係が入るまでは待機し、`bun run install:locked` 完了後に dev server を開始する。これにより、VS Code Dev Container と GitHub Codespaces のどちらでも同じ構成を使える。

ローカルの既定では bind mount 上の所有者と合わせるため `uid/gid 1000:1000` で動かす。GitHub Actions では runner 側 checkout の所有者とずれるため、workflow から `E2E_SERVICE_USER=0:0` を渡して起動する。ローカルでこの上書きが必要になるのは、意図的に同じ条件を再現したい場合だけである。

## 2. 基本手順

テスト実行時のデータは `data_e2e` に分離し、通常の E2E 実行では再生成しない。初回だけ fixture を初期化する。すでに `data_e2e` が揃っている場合、`e2e:fixture:init` は何もしない。E2E 経路は `novel-fetcher-e2e` で、`viewer-api-e2e` は `data_e2e/novel-fetcher` をライブラリルートとして読む。

`tests/fixtures/e2e/novel-fetcher/library.sqlite` は git 管理する E2E fixture の正本である。`data_e2e/novel-fetcher/library.sqlite` は `e2e:fixture:init` / `e2e:fixture:rebuild` が正本から用意する作業コピーであり、E2E service が触っても git 差分として扱わない。

fixture builder の既定は通常 E2E 用作品だけを生成する。`--work-set e2e` で通常 E2E 用作品だけ、`--work-set verification` で検証用作品だけ、`--work-set all` で両方を生成できる。通常 E2E fixture は既存 smoke の固定期待と合わせるため検証用作品を含めない。検証用 site は `verification`、現行の検証用作品はキャラクター抽出の本名・別名・役職名・血縁呼称・偽名の混同を調べる「E2E 人物名寄せ検証」である。

Dev Container の `viewer-dev` には SQLite CLI (`sqlite3`) も入っているため、正本の `tests/fixtures/e2e/novel-fetcher/library.sqlite`、作業コピーの `data_e2e/novel-fetcher/library.sqlite`、`data_e2e/state/ai_usage.sqlite` の調査に使える。

```bash
bun run e2e:fixture:init
```

次に Bun 依存関係をインストールする。

```bash
bun run install:locked
```

E2E 用常駐サービスを手動で起動し直す場合:

`viewer-dev` に attach して実行すると、起動中の Dev Container Compose プロジェクトを再利用し、既存イメージの再ビルドは行わない。VS Code Dev Container / GitHub Codespaces では、起動時に Docker socket proxy を補正するため、通常は `sudo` を付けずにそのまま実行できる。

```bash
bun run e2e:services:up
```

`e2e:services:up` は fixture が未作成のときだけ `data_e2e` を初期化する。必要な E2E service image がまだ存在しない場合は、自動で `docker compose up -d --build` に切り替えて再試行する。Go cache 用 named volume は、E2E サービス起動前に `go-cache-init` が `E2E_SERVICE_USER` に合わせて初期化する。

通常の既定経路は次のとおり:

```bash
bun run e2e:test:container
```

ローカルから環境判定込みで実行する場合:

```bash
bun run e2e:test
```

ブラウザをローカルに入れて直接実行したい場合だけ:

```bash
bun run e2e:test:local
```

`e2e:test` / `e2e:test:local` / `e2e:test:container` は実行前に `data_e2e/state` を初期化する。作品 fixture は保持される。

TTY なしのローカル実行環境では、`e2e:test:container` が `script` を使って疑似 TTY を挟み、Playwright の通常の進捗表示を見やすく保つ。CI や GitHub Actions ではこの補正を行わない。

`e2e:services:up` は `viewer-api-e2e` と `novel-fetcher-e2e` を起動する。AI 生成は `viewer-api` の internal AI module が扱う。

`novel-fetcher-e2e` と `viewer-api-e2e` は既定では `golang:1.25.12-bookworm` 上で dev 起動する。各 service の E2E binary path と image を指定すると、事前 build した binary を軽量 runtime image 上で起動できる。GitHub Actions の Playwright E2E job では、同じ workflow run で build した両 service の binary artifact と `busybox:1.37` を使い、Go toolchain image の pull を避ける。`Service API contract` job は web と Playwright を起動せず、既定の Go dev container で `viewer-api` と `novel-fetcher` だけを起動する。

`playwright-e2e` は `ghcr.io/iuill/narou-viewer-playwright` の image を base にする。image は公開リポジトリ [`iuill/narou-viewer-playwright-images`](https://github.com/iuill/narou-viewer-playwright-images) で daily build し、Chromium headless shell / WebKit / Bun / curl / Noto CJK fonts だけを含める。full Chromium と Firefox を含まないため、公式 Playwright image より CI の pull cost を抑えられる。既定 tag は `.devcontainer/docker-compose.yml` の `PLAYWRIGHT_IMAGE_VERSION` で管理し、`latest` ではなく `@playwright/test` と揃う固定 tag を使う。`@playwright/test` を更新するときは image 側の Playwright version とあわせて更新する。

library smoke suite (`e2e/library-smoke-*.spec.ts`) では `viewer-api-e2e` の `data_e2e/state` を複数シナリオで共有するため、既定では reader の自動既読保存を無効にしている。既読位置そのものを検証するシナリオだけが明示的に自動保存を有効化し、他シナリオの遅延保存が `reading_state.yaml` を上書きして干渉するのを防ぐ。状態変更系の spec は専有 fixture 作品を使い、runner script 側は CI で workers を 2 に制限して同一 project 内の並列実行を許可する。GitHub Actions 側は `pc-xga` と `iphone-16e` の 2 job に分け、各 project の suite 全体を流す。

## 3. GitHub Codespaces での注意

このリポジトリの Dev Container は `runServices` により、Codespaces 起動時点で次の E2E 用サービスも一緒に起動する。

- `novel-fetcher`
- `novel-fetcher-e2e`
- `viewer-api-e2e`
- `viewer-web-e2e`

Codespaces でも VS Code Dev Container でも、通常は次の 1 コマンドだけで E2E を実行できる。`e2e:test:container` 側で fixture 初期化済みの E2E サービスを再利用し、必要なら `e2e:services:up` で補完する。

```bash
bun run e2e:test:container
```

Codespaces 環境の Docker ホストが古い API バージョンを使用している場合、`docker compose` 実行時に次のエラーが出ることがある。

```text
Error response from daemon: client version X.XX is too new. Maximum supported API version is Y.YY
```

その場合は、`DOCKER_API_VERSION` を明示して再実行する。

```bash
export DOCKER_API_VERSION=1.43
bun run e2e:test:container
```

サポートされている API バージョンは `docker version --format '{{.Server.APIVersion}}'` で確認できる。

`viewer-web-e2e` / `viewer-api-e2e` は長時間起動したまま再利用されるため、`vite.config.ts` や dev server の起動オプションを変えた直後は古い設定を保持したまま動き続けることがある。たとえば `Blocked request. This host ("viewer-web-e2e") is not allowed.` のように旧設定が見える場合は、次で E2E 用サービスを立ち上げ直す。

```bash
bun run e2e:services:down
bun run e2e:test:container
```

Codespaces では E2E 用 sidecar の公開ポートを `forwardPorts` に固定登録していない。E2E 実行自体はコンテナ間通信で完結するため問題ないが、必要に応じて VS Code / Codespaces 側で `15173` や `18080` を手動 forward する。複数の git worktree を別 Dev Container として同時起動する場合は、2 つ目以降の worktree で `.devcontainer/.env.example` を `.devcontainer/.env` にコピーし、E2E 用の host port も重複しない値へ変更する。

Codespaces 判定には、GitHub が標準で設定する環境変数 `CODESPACES=true` を使うのが簡単である。必要に応じて `CODESPACE_NAME` や `GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN` も利用できる。

## 4. ターゲットと描画差

Playwright のターゲット環境定義は [`playwright.targets.ts`](../../playwright.targets.ts) に集約している。現在の既定ターゲットは次の 2 つである。

- `pc-xga`: Chromium / `1024x768`
- `iphone-16e`: WebKit によるモバイル端末エミュレーション

現在採用している `@playwright/test` にも `iPhone 16e` の built-in preset がないため、`iphone-16e` は `iPhone 14` の device profile を土台にしている。後からターゲットを変更・追加する場合も、このファイルの配列を編集すれば反映される。

縦書きスクリーンショットではブラウザエンジン差が残る。

- `playwright-e2e` は Noto CJK fonts を含む軽量 Playwright image を使い、日本語の字形差を抑えている。
- それでも `pc-xga` の Chromium と `iphone-16e` の WebKit では、句読点、罫線記号、三点リーダなどの見え方が一致しないことがある。
- 現在は reader 表示に限り、WebKit 系エンジンだけ `text-orientation: mixed` を使っている。

特定ターゲットだけを実行したい場合:

```bash
bun run e2e:test -- --project=pc-xga
bun run e2e:test:local -- --project=pc-xga
bun run e2e:test:container -- --project=iphone-16e
```

## 5. Smoke

本番 URL 向けの read-only smoke test:

```bash
bun run e2e:test:smoke
bun run e2e:test:smoke:container
```

`e2e:test:smoke` / `e2e:test:smoke:container` は [`e2e/prod-readonly-smoke.spec.ts`](../../e2e/prod-readonly-smoke.spec.ts) だけを実行し、state 初期化は行わない。`PLAYWRIGHT_BASE_URL` を指定して外部 URL を検証する用途は `e2e:test:smoke` を正規経路とする。Dev Container / Codespaces 内では `e2e:test:smoke` の既定接続先も `http://viewer-web-e2e:15173` へ自動で切り替わる。`e2e:test:smoke:container` は `playwright-e2e` だけを起動するローカル用ショートカットである。

CI の常設確認はアプリ単体の Playwright E2E を中心に行う。公開入口の TLS / 認証を含む browser E2E は配置先の前段 proxy や運用方針に依存するため常設せず、必要時に `e2e:test:smoke` で外部 origin の read-only 確認を行う。

## 6. 成果物とキャッシュ

初回の `bun run e2e:test:container` では、`playwright-e2e` コンテナ内の専用 volume に `@playwright/test` を展開する。以後はそのキャッシュを再利用するため、Windows bind mount 上の `node_modules` を毎回たどらない。

テスト実行後の `playwright-report` と `test-results` は workspace 側へ同期され、呼び出し元ユーザーで再利用できる所有者へ合わせる。スクリーンショットは成功時・失敗時の両方で保存される。Playwright trace も既定で常時保存する。

一方、GitHub Actions 上では artifact 消費量を抑えるため、`GITHUB_ACTIONS=true` のときだけ `screenshot: "only-on-failure"` と `trace: "retain-on-failure"` を既定に切り替える。必要なら `PLAYWRIGHT_SCREENSHOT_MODE` / `PLAYWRIGHT_TRACE_MODE` で明示的に上書きできる。

GitHub Actions の既定 E2E では、`pc-xga` と `iphone-16e` を project ごとの 2 job として並列実行する。各 job は独立した GitHub-hosted runner 上で `data_e2e/state` を初期化してから実行するため、job 間で同じ state を共有しない。

- 生の成果物: `test-results/<target>/...`
- HTML レポート: `playwright-report/<target>/index.html`

現在の既定ターゲットでは、`test-results/pc-xga` / `test-results/iphone-16e` と `playwright-report/pc-xga` / `playwright-report/iphone-16e` に分かれて保存される。

## 7. 復旧・補助コマンド

fixture を明示的に作り直したい場合:

```bash
bun run e2e:fixture:rebuild
```

`bun run e2e:seed` はこの再生成コマンドの別名である。

検証用作品だけを `data/novel-fetcher` へ追加・更新したい場合:

```bash
cd services/novel-fetcher
go run ./cmd/e2e-fixture-builder --output ../../data/novel-fetcher --work-set verification
```

state だけを手動で初期化したい場合:

```bash
bun run e2e:state:reset
```

`data_e2e` の所有者や権限が壊れた場合:

```bash
bun run e2e:repair
```

`e2e:repair` は既定で `busybox:1.37` の一時コンテナを使い、`data_e2e` の `chown` / `chmod` だけを行う。必要なら `E2E_REPAIR_IMAGE` で image を上書きできる。

停止:

```bash
bun run e2e:services:down
```

ローカルの Dev Container で Docker 公開ポートを直接使う場合の既定ポート:

- `viewer-web`: `5173` (`VIEWER_WEB_HOST_PORT`)
- `viewer-api`: `8080` (`VIEWER_API_HOST_PORT`)
- `viewer-web-e2e`: `15173`
- `viewer-api-e2e`: `18080`

`viewer-web-e2e` と `viewer-api-e2e` の host port は、それぞれ `VIEWER_WEB_E2E_HOST_PORT` と `VIEWER_API_E2E_HOST_PORT` で変更できる。
