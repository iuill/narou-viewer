# 過剰品質削減 設計案: state 保護スタックと sensitive-information 検査基盤

この文書は、個人利用・個人開発 OSS としての narou-viewer に対して過剰になっている品質対策を削減するための設計案である。判断基準は [`quality-goals.md`](quality-goals.md) の「品質投資の判断」に従う。

- 優先して守るリスク: データ消失、機微情報漏えい、外部課金の再発生、互換性破壊、主要機能の回帰
- 削減してよいもの: 単純な運用手順や既存の外部基盤で代替できる application 固有の仕組み、低減するリスクより維持コストが大きい既存の仕組み

`quality-goals.md` の「現行との差分」表は backup / restore の簡素化を既に目標として掲げている。本設計はその実装計画にあたる。

## 1. 対象と現状規模

### 1.1 state 保護スタック

| 構成要素 | 規模（test 含む） | 主な内容 |
| --- | --- | --- |
| `internal/statebackup` + `cmd/state-backup` | 約 5,200 行 | age 暗号化 archive、manifest、2 回復号照合、restore staging、durable restore journal、rollback recovery、retention |
| `internal/statedoctor` + `cmd/state-doctor` | 約 2,550 行 | 横断診断（schema / SQLite / orphan / frontier / file mode）、finding ID 指定の限定 repair、novel-fetcher 契約値の二重管理 |
| `internal/statesecurity` | 約 320 行 | YAML alias 循環対応の raw 平文 `api_key` 検査（backup preflight / doctor 専用） |
| `internal/statebarrier` + novel-fetcher `internal/writerlock` | 約 435 行 | writer lock と restore journal による起動拒否 |
| docs | 約 640 行 | `state-backup.md`、`state-doctor.md`、`state-schema-policy.md` の backup / restore / 診断章 |

### 1.2 sensitive-information 検査基盤（Secret Guard）

| 構成要素 | 規模 | 主な内容 |
| --- | --- | --- |
| `sensitive-information-events.yml` + `sensitive-information.yml` | 約 320 行 | `pull_request_target` → artifact 受け渡し → 専用 GitHub App token による commit status 発行、freshness 照合、head SHA を共有する全 PR の metadata scan |
| status / PR 解決系 script 7 本 | 約 160 行 | `update-sensitive-status.sh`、`assert-latest-sensitive-status.sh`、`resolve-pull-request-*.sh`、`list-pull-requests-for-head.sh`、`scan-pull-request-{content,metadata-for-head}.sh` |
| `scan-sensitive-changes.sh` | 233 行 | staged / message / branch / range / pre-push / history の各 mode、binary blob の strings 抽出検査、remerge diff 検査 |
| `test-sensitive-information-checks.sh` | 574 行 | fake betterleaks / fake gh を使う回帰テスト |
| その他 | 約 130 行 | `check-sensitive-paths.sh`、`check-sensitive-content.sh`、`install-betterleaks.sh`、`.githooks/`、`security-audit.yml` の history scan |

## 2. 設計方針

残すものと削るものを、想定する脅威・障害の現実性で分ける。

残す（優先リスクへ直接効く、実行時に働く、行数が小さい）:

- 読み書き時の fail-closed な schema version guard、atomic write、fail 時 zero-mutation
- AI credential の暗号化と lazy migration
- checkpoint fingerprint fence 等の課金再発防止
- 開発者自身の secret / 私的情報の commit 混入を防ぐローカル検査（hooks + betterleaks）と CI での検査

削る（攻撃者・並行運用者・大規模運用を想定した多層防御と専用 tooling）:

- 専用 backup / restore tooling（暗号化 archive、manifest、restore journal、rollback recovery、retention）
- 横断診断 CLI と限定 repair
- fork PR による status 偽装・レースを想定した GitHub App ベースの status 発行基盤
- binary blob からの strings 抽出 scan などの低頻度経路

## 3. state 保護スタックの削減設計

### 3.1 statebackup CLI の撤去

`quality-goals.md` は「writer 停止中の data root 全体 copy」を基準経路と定めている。専用 tooling は撤去し、手動手順に置き換える。

削除対象:

- `apps/viewer-api-go/internal/statebackup`（全 file、約 4,760 行）
- `apps/viewer-api-go/cmd/state-backup`（約 470 行）
- `deploy/viewer-api-go/Dockerfile` の `state-backup` build / COPY
- `package.json` の `state:backup`
- `docs/state-backup.md`（手動手順に置換）

代替手順（`deployment.md` へ 20 行程度で記載）:

```bash
docker compose -f docker-compose.prod.yml stop viewer-api novel-fetcher
tar czf backup-$(date +%Y%m%d).tar.gz -C <data-root> .
# 任意: 保存先が暗号化されていない場合
age -r 'age1...' -o backup-...tar.gz.age backup-...tar.gz
docker compose -f docker-compose.prod.yml start novel-fetcher viewer-api
```

restore は「service 停止中に空の data root へ backup 全体を展開する」だけとする。retention・保存先・暗号化は運用者と外部基盤（暗号化 disk、cloud storage の versioning 等）の責任とする。

既存 archive の互換性: 現行 archive は age(gzip(tar)) の標準形式であり、tooling なしで `age -d -i <identity> <archive> | tar xz -C <empty-data-root>` により復元できる。manifest.json は archive 内に残るが読み飛ばしてよい。この 1 行を移行ノートとして docs に残す。

### 3.2 restore journal と起動拒否の撤去、writer lock の維持

restore journal（`data/.state-restore-transaction.json`）は restore tooling の staging / rollback 専用であり、tooling 撤去で存在しなくなる。

- `statebarrier.EnsureNoRestoreInProgress` / novel-fetcher `writerlock.EnsureNoRestoreInProgress` と両 main からの呼び出しを削除する。
- writer lock（`AcquireViewerAPI` / `Acquire`）は維持する。二重起動による YAML CAS state の破壊を防ぐ実行時 guard であり、約 100 行で維持コストがほぼない。

### 3.3 statedoctor の撤去

削除対象:

- `apps/viewer-api-go/internal/statedoctor` + `cmd/state-doctor`（約 2,550 行）
- `docs/state-doctor.md`、`package.json` の `state:doctor`、Dockerfile の `state-doctor`

根拠:

- 実行時 guard が read / write 時点で同じ異常を fail-closed に検出し、path・observed / supported version・復旧案を含む error を返す。doctor は同じ契約の再実装である。
- 派生 state（character profile、job index、reader search）は実行時に quarantine / rebuild されるため、限定 repair の対象は runtime で自動回復済みである。
- novel-fetcher の migration version / canonical episode version を doctor 側にも持つ二重管理契約があり、schema を変更する全 PR に契約値と fixture の同時更新という恒常コストがかかる。

代替:

- 起動ログ（既存の fail-closed error）を一次診断とする。
- SQLite の健全性確認は `sqlite3 <db> 'PRAGMA quick_check'` の手動手順を `deployment.md` に記載する。

段階案として repair / reconcile だけ落として read-only scan を残す縮小もあり得るが、二重管理契約が残り恒常コストが解消しないため推奨しない。

### 3.4 statesecurity の撤去と起動時 warning への置換

`statesecurity` の利用箇所は backup preflight と doctor のみで、両者の撤去により dead code になる。alias 循環まで扱う raw YAML walker は削除する。

legacy 平文 `api_key` への対策は次で置き換える:

- 既存の lazy migration（master passphrase 設定時に encrypted payload へ移行）を維持する。
- `aisettings` repository が typed load で document を読む際、非空の平文 `api_key` を検出したら warning log を 1 行出す（値は出力しない）。数行の追加で済み、backup 時ではなく毎起動時に気付ける。

### 3.5 維持するもの

以下は削減対象にしない。優先リスクへ直接対応し、いずれも小さい。

- `schemaguard` / `safefile` / `yamlfile` / `filequarantine` と各 repository の version fence
- SQLite の番号付き migration と future-version guard（`ai_usage.sqlite`、`library.sqlite`）
- `reader_search.sqlite` の quarantine / rebuild
- extraction checkpoint の generation fingerprint fence（課金再発防止）
- `aisettings` の crypto 実装
- novel-fetcher task queue の起動 recovery

### 3.6 ドキュメント再編

- `state-schema-policy.md`: §6（backup / restore / rollback、約 60 行）を「writer 停止中の data root 全体 copy と、旧 data + 旧 build の組での rollback」数行に置換する。§5.3 の doctor 前提の診断記述、§2.6 の backup 記述、§7 checklist の backup / doctor 由来項目（archive / manifest / 暗号化 / retention / doctor 契約値更新）を削除する。registry 本体（§1〜§4）と version 運用は維持する。
- `docs/README.md` から `state-doctor.md` / `state-backup.md` を外し、手動 backup 手順への参照を `deployment.md` に一本化する。
- `quality-goals.md` の「現行との差分」表の backup / restore 行を実装済みとして更新する。

### 3.7 移行手順

1. 移行前に現行 build で最後の backup を取得する（tooling でも手動 copy でもよい）。
2. viewer-api / novel-fetcher の startup から `EnsureNoRestoreInProgress` を外し、tooling・doctor・statesecurity を削除する。
3. docs を再編し、既存 `.tar.gz.age` の手動復元 1 行を移行ノートに残す。
4. 削除はすべて git 履歴に残るため、実運用で不足が判明した場合は個別に復活を検討できる。

なお `quality-goals.md` 差分表には AI usage の最小 request ledger 化も残っているが、本設計の対象外とし別課題として扱う。

## 4. sensitive-information 検査基盤の削減設計

### 4.1 GitHub App「Secret Guard」基盤の廃止

現行の 2 workflow + App 構成は、fork PR が checkout や status 発行を悪用して required check を偽装・レースさせる攻撃への対策である。具体的には `pull_request_target` から artifact 経由で PR 番号だけを渡し、App token で status を発行し、freshness 照合と head SHA を共有する全 PR の metadata scan まで行う。

個人開発 repo ではこの脅威に投資する妥当性がない。fork PR の commit 検査は、secrets を一切持たない通常の `pull_request` event で同じ scan を実行すれば足りる（`GITHUB_TOKEN` は read-only、App token 不要、status API 不要）。

新構成（独立 workflow 1 本、または既存 CI への 1 job 追加）:

```yaml
name: Sensitive Information
on:
  pull_request:
jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<pinned>
        with:
          fetch-depth: 0
      - run: bash ./scripts/scan-sensitive-changes.sh range \
          "${{ github.event.pull_request.base.sha }}" \
          "${{ github.event.pull_request.head.sha }}"
```

- required check は commit status `sensitive-information/commits` からこの job の check run へ branch protection 設定で差し替える。
- PR metadata（本文・comment・review）の advisory scan は廃止する。個人 repo で第三者 metadata 経由の混入リスクは低く、commit 履歴と異なり検出後に編集で除去できる。必要になれば `workflow_dispatch` の手動 scan として最小構成で復活できる。

削除対象:

- `.github/workflows/sensitive-information.yml`、`.github/workflows/sensitive-information-events.yml`
- `scripts/update-sensitive-status.sh`、`assert-latest-sensitive-status.sh`、`resolve-pull-request-number.sh`、`resolve-pull-request-event.sh`、`list-pull-requests-for-head.sh`、`scan-pull-request-metadata-for-head.sh`、`scan-pull-request-content.sh`

後片付け:

1. 新 workflow を追加して green を確認する。
2. branch protection の required check を差し替える。
3. 旧 workflow / script を削除する。
4. Secret Guard GitHub App を uninstall し、`SECRET_GUARD_APP_CLIENT_ID` var と `SECRET_GUARD_APP_PRIVATE_KEY` secret を削除する。
5. `AGENTS.md`、`SECURITY.md`、`CONTRIBUTING.md` の Secret Guard / advisory 記述を更新する。

### 4.2 scan-sensitive-changes.sh の縮小

- binary blob 検査（`scan_binary_commits` / `scan_raw_blob` / `scan_binary_blob_if_needed`、約 65 行）を削除する。この repo にバイナリで secret が入る現実的経路は細く、鍵・証明書系の代表的ファイル名は `check-sensitive-paths.sh` が遮断し、テキスト diff は betterleaks が検査する。
- pre-push mode の専用 while ループを簡素化し、`<local_sha> --not --remotes=<remote>` を対象にした paths check + added-content check + betterleaks 1 呼び出しに統一する（commit message scan は維持）。
- 維持: staged / message / branch / range / history の各 mode、`--ignore-gitleaks-allow`、公開 IPv4 検査、betterleaks の version pin と自動 install（いずれも小さく、検出力か再現性に直結する）。

### 4.3 テストハーネスの縮小

`test-sensitive-information-checks.sh`（574 行）から、status 発行 script 用の fake gh、advisory / freshness 系、binary blob 系のケースを削除し、次の代表ケースに絞る（目安 200 行以下）:

- 禁止 path 検出（`data/` 配下、`.env`、秘密鍵ファイル名）
- 公開 IPv4 検出と reserved range の非検出
- staged / range scan での secret 検出と clean pass
- allow marker が無効化されていることの確認

`AGENTS.md` の「scanner 変更時は `bun run test:security`」の運用は維持する。

### 4.4 維持するもの

- `.githooks/` の pre-commit / commit-msg / pre-push（入口の統一は維持）
- `check-sensitive-paths.sh`、`check-sensitive-content.sh`
- `security-audit.yml` の週次 history scan と dependency audit

## 5. 削減効果の見積もり

| 領域 | 削減規模 |
| --- | --- |
| Go（statebackup + statedoctor + statesecurity + journal 連動） | 約 8,100 行（test 含む） |
| shell / workflow（Secret Guard 基盤 + scan / test 縮小） | 約 900 行 |
| docs | 約 300 行（縮約後の新規手順 30〜40 行を差し引き） |
| 恒常コスト | schema 変更 PR ごとの doctor 契約値・fixture 同時更新、App key の管理・rotation、status レース考慮が不要になる |

## 6. リスクと割り切り

- 復旧の粒度は backup 取得頻度に依存する。upgrade 前 backup の必須運用は維持する。
- backup の暗号化・retention は運用者責任になる。docs に age の 1 行手順を示して補う。
- fork PR の metadata 経由の混入は自動検知しなくなる。編集で除去可能なため許容する。
- 横断診断がなくなるため、複数 file にまたがる不整合の一括レポートは得られない。実行時 error と手動 `quick_check` で代替する。

## 7. 実施ステップ（PR 分割案）

1. sensitive-information: 新 `pull_request` workflow 追加 → required check 差し替え → 旧 workflow / script / App 撤去、scan と test の縮小、関連 docs 更新
2. statebackup + restore journal の撤去、手動 backup / restore 手順の `deployment.md` 追記、`state-backup.md` 削除
3. statedoctor + statesecurity の撤去、`aisettings` 起動時 warning 追加、`state-doctor.md` 削除
4. `state-schema-policy.md` 縮約と `quality-goals.md` 差分表の更新（3 と同一 PR でもよい）

各 PR は独立して merge / revert できる。1 は GitHub 設定変更（branch protection、App uninstall）を伴うため、設定手順を PR 本文に明記する。
