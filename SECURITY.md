# Security Policy

セキュリティ上の問題を見つけた場合は、公開 issue に secret や取得済み本文を貼らず、GitHub の private vulnerability reporting が利用できる場合はそちらから報告してください。利用できない場合は、本文や secret を含まない最小限の概要だけを issue で共有し、詳細な再現情報の受け渡し方法を maintainer と調整してください。

## 報告対象

- path traversal、任意ファイル読み取り・書き込み
- SSRF や、想定外の外部 URL へアクセスできる問題
- API key、cookie、private key、`.env.local`、非公開運用情報の漏えい
- 外部サイトへの過剰アクセスを誘発する不具合
- 取得済み本文、raw HTML、画像など第三者作品データの混入
- 利用者の明示的な opt-in なしに本文や抜粋が外部 AI provider へ送信される問題

## 報告時のお願い

- 第三者作品本文、取得済み raw HTML、取得済み画像は添付しないでください。
- 再現には synthetic fixture、最小の自作データ、または本文引用を含まない操作手順を使ってください。
- secret を含むログを共有する場合は、送信前に必ず redaction してください。
