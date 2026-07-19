# narou-viewer 品質目標

この文書は、narou-viewer の変更判断で優先する品質目標を定める正本である。
現行の実装事実と機能契約は [`architecture.md`](architecture.md) と各機能仕様を基準とし、この文書の未実装目標を現行保証として扱わない。
目標を実装する変更では、関連する機能仕様、運用手順、テスト、互換性説明を同じ変更で更新する。

## 品質投資の判断

narou-viewer は個人利用者向けの個人開発 OSS である。
品質対策は、障害の影響、発生可能性、検出可能性と、実装、運用、保守にかかるコストの釣り合いで判断する。

次のリスクは優先して対策する。

- 利用者が再構築できないデータの消失や破損
- 機微情報や第三者作品データの漏えい
- 依存関係や build tool の更新経路から混入する悪意あるコード
- 意図しない外部 request、重複処理、課金の再発生
- upgrade によるデータ互換性の破壊
- 日常的な読書を妨げる主要機能の回帰

次の仕組みは、具体的な利用要件や障害実績がない限り導入しない。

- multi-user isolation、active-active、HA、無停止 deployment
- 個人利用では使わない広範な互換性 window
- 外部基盤や単純な手動手順で代替できる専用 backup automation
- 変更頻度と障害の影響に見合わない常設 test matrix
- 単純な手動復旧より保守負担が大きい自動 recovery

過剰品質を避ける方針は、データ保護、秘密情報、課金境界、schema compatibility を省略する理由にはしない。
単純な運用手順や既存の外部基盤でリスクを十分に下げられる場合は、application 固有の複雑な仕組みを増やさない。
既存の仕組みも、低減するリスクより維持コストが大きい場合は、互換性と移行手順を整理したうえで簡素化できる。

## 利用形態

- `single-user / single-deployment` を前提とする。
- PC、smartphone、tablet など、複数ブラウザ、複数端末からの利用をサポートする。
- 永続 state の owner ごとに active writer は 1 instance とする。
- multi-user isolation、active-active、shared storage を使った水平 scale、automatic failover、HA はサポートしない。
- 公開時の TLS と認証は reverse proxy、VPN、tunnel、hosting platform などの外部基盤で提供する。

複数端末対応は複数 writer instance を意味しない。
browser client は複数存在できるが、各永続 state を更新する server process は一つに限定する。

## upgrade と互換性

- 永続 schema は upgrade 方向だけをサポートする。
- 各 build は、直接読み取るか migration できる旧 schema version を明示する。
- 最低限、直前の schema version から current schema version への直接 upgrade を保証する。
- 必要な migration chain が実装されている範囲では、release を飛ばした upgrade を許容する。
- 未対応の schema version からの upgrade は fail-closed で拒否し、元の file や DB を変更しない。
- 新しい build が使用した data directory を古い build で開く in-place downgrade はサポートしない。
- rollback では upgrade 前に取得した data root 全体の backup と、それに対応する旧 build を使用する。

「downgrade 非対応」は application binary だけを戻して同じ data directory を使う操作を指す。
upgrade 前の data と旧 build を一緒に戻す rollback は、対応する復旧手順として扱う。

## 永続化

- 単一 file の更新は temp file、同期、rename を使って atomic に公開する。
- SQLite の更新は transaction 内で実行する。
- 複数 file や複数 DB を跨ぐ global transaction は保証しない。
- multi-file 更新は正本と派生データを分け、保存順、commit 境界、再生成方法を定義する。
- 外部 request や課金が再発生し得る中断状態は、推測で自動再実行しない。

## backup と restore

- application による自動 backup は品質目標に含めない。
- 運用者が `viewer-api` と `novel-fetcher` を停止し、共有 data root 全体を backup する方式を基準とする。
- cron、host の snapshot、外部 backup software などによる自動化は妨げない。
- 稼働中の file copy、DB や作品単位の部分 backup は正式な復旧手段としてサポートしない。
- restore は service 停止中に空の data root または新しい volume へ backup 全体を復元する。
- 現在の data を残したまま上書きする restore は正式な復旧手段としてサポートしない。
- 外部 backup の copy 完了性は、運用者と backup 基盤が管理する。
- application は起動時または state の読み書き時に、存在する state の schema、SQLite の必須 table / column、migration version を検査するが、SQLite database 全体の integrity は保証しない。
- SQLite database 全体の integrity は、writer 停止中の診断または restore 後の確認で `PRAGMA quick_check` を実行して判定する。
- backup の定期実行、保存先、暗号化、retention は運用者または外部 backup 基盤が管理する。
- upgrade 前の backup は必須手順とする。
- credential、master passphrase、backup の復号情報は data backup と分けて管理する。

data root 全体を扱うのは、`library.sqlite` と `works/**`、AI の生成正本と checkpoint などが相互に対応するためである。
一部だけを過去へ戻すと、DB が存在しない file を参照したり、完了済みの外部 request を再実行したりする可能性がある。

専用 archive、manifest、自動 restore、restore journal は提供しない。
writer 停止中の data root 全体 copy を基準となる復旧経路とする。

## 読書位置

- 読書位置は作品単位の version を使った条件付き更新とする。
- client が最後に確認した version と server の version が一致する場合だけ保存する。
- 古い version を基準にした保存は自動適用せず、競合として扱う。
- 同一端末による同一位置の重複保存と、表示位置と server 位置が同じ場合は自動解決する。
- 本文表示中に別端末の新しい保存位置が現在位置と異なる場合は、自動保存を止めて利用者に選択を求める。

競合時は、別端末の位置を反映するか、この端末の位置で上書きするかを選択できるようにする。
話数の大小だけでは、読み返しと意図しない巻き戻しを区別できないため、自動的に進んでいる側を採用しない。

## AI データ

- 外部 provider の再実行が必要で、結果が非決定的または有償となる生成結果は、必要性に応じて生成正本として保護する。
- 生成正本からローカルで再構築できる表示用 profile、index、search data は派生データまたは cache として扱う。
- job と checkpoint は、重複 request、重複適用、課金再発生を防ぐために必要な最小限の運用 state とする。
- schema や generation 条件が一致しない job と checkpoint は自動 resume しない。
- 具体的な正本形式、投影形式、commit 境界は各機能仕様で定義する。

生成正本は再生成できる場合があっても、同じ結果になる保証がなく、provider request と料金も再発生し得る。
したがって、再生成可能性だけを理由に cache として破棄しない。

## AI usage

- AI usage は prompt、回答、小説本文、tool I/O の内容を保存しない最小 request ledger とする。
- ledger には run / request の識別、feature、provider、model、状態、retry 関係、token、cost などの利用 metadata だけを保存する。
- provider の raw response、credential、内容を含む raw error message は保存しない。
- request 数、token、cost の aggregate は ledger から算出する。
- aggregate を別の正本として手動更新しない。
- aggregate cache を設ける場合は ledger から再生成可能にする。

request 単位の ledger を残すことで、aggregate だけでは判別できない retry、二重計上、部分失敗を確認できる。
内容を保存対象から外すことで、利用状況の確認に不要な本文や会話を server state に残さない。

## CI

- application CI は pull request、main push、manual dispatch を扱う 1 workflow とする。
- web build、repository lint / TypeScript coverage、各 Go service の検証 / build、API contract、Playwright E2E は役割別の job に分け、artifact 依存を除いて並列実行する。
- API contract の通常 suite は独立した job の fixture 専用環境で 1 回だけ実行し、各 service の検証 job では重複実行しない。取得 backend の destructive contract も同じ job で別途明示的に実行する。
- security、dependency、権限境界や起動 event が異なる検査は application CI から分離する。
- 依存関係の検査では既知の脆弱性だけでなく、公開直後の version を取り込む supply-chain risk も扱う。
- Go module は、緊急修正などで version 単位の例外を明示した場合を除き、公開から 21 日経過するまで採用を拒否する。
- release-age 検査は安全性の保証や脆弱性監査の代替とはせず、dependency review と既知脆弱性監査を併用する。
- 同一 repository の branch から作成した pull request では、repository 全体と base からの規模差分を専用 workflow で計測し、変更規模、増加した領域、意図しない生成物や責務の膨張がないかを maintainer が確認する。
- fork 由来の pull request では書き込み権限を持つ repository size workflow を実行せず、maintainer が差分から同じ観点を確認する。
- repository size report は自動的な行数上限にはせず、必要な機能追加を妨げない review 材料として扱う。
- repository size report の pull request 書き込み権限は application CI から分離した専用 workflow に限定する。
- application E2E の schedule による定期実行は行わない。
- pull request と main push では、`pc-xga` Chromium と `iphone-16e` WebKit で `e2e/` 配下の Playwright E2E suite 全体を実行する。
- manual dispatch でも同じ 2 browser の Playwright E2E suite 全体を実行できるようにする。
- 常設する browser、viewport、OS の matrix はこの 2 target に限定し、具体的な利用要件や障害実績がない限り追加しない。

「1 workflow」は一つの job へ直列化する方針ではない。
PR と main で重複する application CI の定義を共有し、検査ごとの並列性と required check は維持する。

## coverage と重点テスト

個人開発では継続的な人的 review の量を増やしにくいため、自動テストと高い coverage を主要な回帰検知手段とする。

- repository 全体を一つの coverage 値で評価しない。
- `viewer-web`、`viewer-api`、`novel-fetcher` 単位の coverage gate は、意図的な品質投資として高い水準で維持する。
- threshold は subsystem のリスクとテスト特性に応じて個別に管理し、repository 全体で共通化しない。
- coverage gate の達成だけで品質を満たしたとは判断しない。

重点テストは次の境界へ配置する。

- **parser**：外部 HTML、embedded JSON、URL の変更や malformed input を誤って保存しない。
- **schema**：直前 schema version の migration、未対応 version の拒否、拒否時の zero mutation を確認する。
- **path**：path traversal、absolute path、symlink escape、unsafe ID を data root 外へ通さない。
- **永続化**：atomic write、SQLite rollback、multi-file commit 境界を確認する。
- **backup / restore**：基本 schema と version guard は自動テストで確認する。運用手順を変更した場合は、停止中の全体 copy、新しい volume への restore、SQLite integrity、旧 data と旧 build を組み合わせる rollback 境界を synthetic data で確認する。
- **読書位置**：古い version の拒否、同一位置の自動解決、別端末との競合解決を確認する。
- **API contract**：web、viewer-api、novel-fetcher 間の request、response、status、error を固定する。
- **外部処理**：retry、cancel、idempotency、checkpoint、token / cost 記録の境界を確認する。

## デプロイ

- self-host compose の長時間動作する application container は、`viewer-web`、`viewer-api`、`novel-fetcher` の 3 containers とする。
- `viewer-web` の Nginx は static asset、SPA fallback、Service Worker を配信し、同一 origin の `/api/*` を `viewer-api` へ転送する。
- host へ publish する HTTP port は `viewer-web` だけが持ち、`viewer-api` と `novel-fetcher` は内部 network に留める。
- TLS と認証は application container へ追加せず、外部 ingress、VPN、tunnel、hosting platform などで扱う。
- one-shot の volume 初期化処理は application container 数に含めない。
- static web 配信を `viewer-api` へ統合して Nginx をなくす案は、cache header、SPA fallback、Service Worker と web build の責務を Go application に加える具体的な必要性が生じた場合だけ再検討する。

## 現行との差分

| 項目 | 現行 | 目標 |
| --- | --- | --- |
| AI usage | run / request metadata に加えて、制限付き tool I/O を含み得る snapshot を保存 | 内容を保存しない最小 request ledger に限定 |

backup と restore の簡素化は実装済みであり、writer 停止中の data root 全体 copy を現行経路とする。
application E2E は、PC と smartphone の主要回帰を継続的に検出する品質投資として、2 browser の現行 suite 全体を pull request と main push で実行する。
表に残る目標は、対応する実装と機能仕様が更新されるまで現行挙動を置き換えない。
