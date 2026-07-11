<!--
各節を具体的に記載してください。該当しない場合は「なし」と理由を記載します。
実施した検証だけをチェックし、未実施・対象外の検証は「未実施の検証」に理由を残します。
-->

# Pull Request

## 概要

## ユーザーへの影響

- 対象となるユーザー・運用者:
- 表示・操作・性能・セキュリティ・プライバシーへの影響:
- 破壊的変更の有無:

## 変更内容

-

## 互換性・移行・ロールバック

- API・設定・環境変数の互換性:
- 永続データ（YAML / SQLite / browser storage / cache）の互換性:
- 移行・backfill の要否と手順:
- ロールバック時の注意点:

## 検証

### 自動検証

- [ ] `bun run lint`
- [ ] `bun run test:unit`
- [ ] `bun run build`
- [ ] `bun run verify:api-go`
- [ ] `bun run verify:api-go:contract`
- [ ] `bun run verify:novel-fetcher`
- [ ] `bun run test:security`
- [ ] `bun run audit:bun:vulnerabilities`
- [ ] `bun run audit:go:toolchain`
- [ ] `bun run audit:go:vulnerabilities`
- [ ] `bun run audit:go:module-age`
- [ ] `bun run e2e:fixture:init` → `bun run e2e:services:up` → `bun run e2e:test:container`

### 手動検証

- [ ] UI 変更を `pc-xga` / `ipad-mini` / `iphone-16e` で確認しました
- [ ] その他の手動確認を実施しました（内容を以下に記載）

### 未実施の検証

<!-- 未実施・対象外のコマンドと、その理由を記載してください。 -->

## ドキュメント・運用

- [ ] 仕様、セットアップ、データ契約、運用手順に関係するドキュメントを更新しました
- [ ] AI 機能で外部 provider へ送信され得るデータを UI / README / docs に明記しました
- [ ] デプロイ後の作業や監視が必要な場合、その手順を記載しました

## 安全性の確認

- [ ] 第三者作品本文、取得済み raw HTML、取得済み画像、実作品由来の model output を含めていません
- [ ] API key、cookie、private key、`.env.local`、個人運用環境の実 IP や secret を含めていません
- [ ] 再現データには synthetic fixture、自作データ、または利用許諾済みデータを使用しました
- [ ] PR は特段の理由がない限り ready for review で起票します

## 関連 Issue・補足

PR タイトル、本文、コメントは原則として日本語で記述してください。secret や第三者作品由来の本文を貼らず、再現情報は synthetic fixture、最小の自作データ、または本文引用を含まない操作手順で説明してください。
