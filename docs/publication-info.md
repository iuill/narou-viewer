# 書籍情報とカバー画像表示

## 目的

各小説について、書籍版・コミック版などの書籍情報とカバー画像を viewer から確認できるようにする。

外部検索結果の誤確定を避けるため、自動探索は控えめに扱う。手動 ISBN 登録を起点に NDL サーチで書誌確認し、Google Books API でカバー URL と不足書誌を補完する。将来的には NDL サーチのタイトル検索から ISBN 候補を自動抽出し、Google Books は ISBN 補完・カバー provider として使う。

現実の書籍化は小説版 / コミック版がそれぞれ 1 系統とは限らない。部ごとの分冊シリーズ、出版社違い、コミカライズ担当変更による再始動、同一作品の新装版などがあり得る。そのため保存モデルは `novel` / `comic` の種別ごとに複数 entry を持てる形にし、ライブラリ一覧で使う代表カバーも entry 単位で選べるようにする。

## Provider 方針

- Primary: NDL サーチ
  - 現行実装では ISBN lookup を行う。
  - 将来の自動探索では、タイトル検索、ISBN 候補抽出、書誌正本寄りの candidate source とする。
  - 自動確定の主 evidence は NDL 由来に寄せる。
- Secondary: Google Books API
  - ISBN lookup から表紙画像と補助書誌を補完する。
  - Google Books API key は `GOOGLE_BOOKS_API_KEY` を必須とする。
  - NDL 候補が出ない場合だけ title search fallback を検討する。
  - Google Books 由来の情報を UI に出す場合は、source 表示、Google Books へのリンク、attribution を表示する。
- Avoid: 楽天ブックス API / openBD
  - 現行実装では採用しない。

## 実装範囲

- `data/state/publications.yaml` に作品ごとの `entries` と `display_cover_entry_id` を保存する。
- `kind` は `novel` と `comic` の 2 種類に限定する。ただし同じ `kind` の entry は複数持てる。
- 作品詳細画面の `書籍情報` タブで、小説版 / コミック版ごとに複数カードを表示する。
- ISBN13 を手動登録すると、サーバー側で NDL サーチ OpenSearch の ISBN lookup を行い、取得できた title / creator / publisher / issued / detail link を書誌情報として保存する。
- NDL の後に Google Books の `q=isbn:<isbn13>` lookup を行い、imageLinks と Google Books link、NDL で不足した補助書誌を保存する。
- NDL / Google Books の一時障害で手動 ISBN 登録自体を失敗させない。外部補完失敗は entry の warning として保存し、取得できた provider metadata だけを表示する。
- NDL の ISBN lookup では、返却 item の ISBN identifier が入力 ISBN と一致する場合だけ NDL 書誌として採用する。
- カバー画像は URL のみ保存し、画像バイナリは保存・再配信しない。
- `PUBLICATION_PROVIDER_NDL_ENABLED=0` の場合は NDL lookup を行わない。
- `GOOGLE_BOOKS_API_KEY` 未設定時は Google Books へ通信せず、Google Books 補完は warning として扱う。
- `GOOGLE_BOOKS_API_KEY` 未設定かつ Google Books provider 有効時は `/api/system/status` でも warning を返し、トップ画面の動作状況 warning として表示する。
- `PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED=0` の場合は Google Books lookup を行わず、ISBN だけを手動情報として保存する。
- `NDL_SEARCH_API_BASE_URL` と `GOOGLE_BOOKS_API_BASE_URL` はテストや検証で endpoint を差し替える場合だけ使う。
- UI では、NDLサーチ API を用いた metadata であることを明記し、Google Books 由来の表紙・書誌情報には Google Books へのリンク付き attribution を表示する。
- ライブラリ一覧では Google Books attribution は一覧単位にまとめ、Google Books 由来の各カバーには出典リンク一覧から個別の Google Books link を持たせる。
- ライブラリ一覧に表示するカバーは、ユーザーが `displayCoverEntryId` として選択できる。未選択時は表紙ありの entry から小説版、コミック版の順に deterministic fallback する。
- 作品詳細では `話 / 書籍情報 / 栞` のタブで補助情報を切り替え、初期表示は読書導線を優先して `話` とする。
- タブレット・スマホではライブラリカードの `書籍情報` ボタンから詳細画面の `書籍情報` タブへ直接移動できる。
- 書籍情報と栞は、50 話単位の話一覧の下に埋もれないよう、同じ位置のタブから切り替えて表示する。

## データモデル

```yaml
novels:
  - novel_id: example
    display_cover_entry_id: novel-9784040000008
    entries:
      - id: novel-9784040000008
        kind: novel
        status: manual
        override: isbn
        isbn13: "9784040000008"
        title: 書籍版タイトル
        image_url: https://example.test/cover.jpg
      - id: comic-9784040000009
        kind: comic
        status: manual
        override: isbn
        isbn13: "9784040000009"
```

entry の `id` は API 操作用の安定 ID として扱う。未登録の小説版 / コミック版枠は保存データには持たず、UI の追加フォームとして表現する。登録時は `kind-isbn13` を起点に ID を生成し、同じ種別の entry を複数保存できる。

## API

```text
GET /api/library/novels/{novelId}/publications
POST /api/library/novels/{novelId}/publications/entries
PUT /api/library/novels/{novelId}/publications/entries/{entryId}
PUT /api/library/novels/{novelId}/publications/display-cover
```

`POST entries` body:

```json
{
  "kind": "novel",
  "mode": "isbn",
  "isbn13": "9784040000008"
}
```

`PUT entries/{entryId}` body:

```json
{
  "kind": "novel",
  "mode": "isbn",
  "isbn13": "9784040000008"
}
```

解除または無効化:

```json
{
  "kind": "novel",
  "mode": "none"
}
```

```json
{
  "kind": "novel",
  "mode": "disabled"
}
```

非表示から再表示:

```json
{
  "kind": "novel",
  "mode": "visible"
}
```

一覧用カバー選択:

```json
{
  "entryId": "novel-9784040000008"
}
```

空文字を送ると明示選択を解除し、fallback に戻す。

## 将来拡張

- シリーズ単位の title / creator / publisher / source と、巻単位の ISBN / cover / provider metadata を分けて扱う。
- NDL サーチのタイトル検索を追加し、タイトル候補から ISBN を抽出する。
- ISBN13 を正本 key とした candidate merger / scorer を追加する。
- 自動確定閾値は、小説版よりコミック版を厳しめにする。
- Google Books title search fallback は既定無効にし、必要になったら `PUBLICATION_GOOGLE_BOOKS_TITLE_SEARCH_ENABLED=1` で有効化する。
- 候補 UI で Google Books title search 結果を出す場合は、NDL 候補と混ぜず provider ブロックを分け、Google attribution と Google Books link を表示する。
