# Contributing

narou-viewer は、個人利用向けの非公式 self-hosted viewer です。issue / PR では、第三者作品の本文や取得済みデータを repository に持ち込まないでください。

## Issue / PR に含めないもの

- 第三者作品の本文、長い引用、取得済み raw HTML
- 取得済み画像、スクリーンショット内の本文全文、実データ archive
- API key、cookie、private key、個人運用環境の実 IP や secret

## 再現情報の書き方

- 再現には synthetic fixture または最小の自作データを使ってください。
- 実サイトの構造変更を報告する場合は、公開 URL、操作手順、期待結果、実際の結果に留め、本文引用は避けてください。
- 外部サイトへ高頻度アクセスする再現手順は避け、保存済み fixture や parser unit test で確認できる形に寄せてください。

## PR / commit / コメント

- コミットメッセージ、Pull Request のタイトルと本文、Pull Request 上のコメントは、原則として日本語で記述してください。
- Pull Request は、特段の理由がない限り draft ではなく ready for review で起票してください。
- Pull Request へ追いコミットした場合は、PR のタイトルや本文が変更内容と乖離していないか確認し、必要なら更新してください。

## 受け付けない変更

- 各サイトの利用規約違反や過剰アクセスを助長する変更
- 取得済み本文、画像、raw HTML の再配布を目的にする変更
- 利用者が明示的に有効化していない cloud LLM provider へ本文や抜粋を送る変更

AI 機能に関する変更では、本文またはその抜粋・要約用テキストが外部 provider に送られる可能性を UI / README / docs で明確に説明してください。
