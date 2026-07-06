---
name: ui-review-screenshot
description: Use when a narou-viewer UI change needs a dedicated visual review phase with playwright-cli across pc-xga, ipad-mini, and iphone-16e, separate from normal E2E verification.
---

# UI Review Screenshot

この skill は、UI 変更時に「E2E とは別の画面確認フェーズ」を `playwright-cli` コマンド中心で回すための最短手順です。正本のルールは [`AGENTS.md`](../../../AGENTS.md) を優先します。

## 基本方針

- UI を変えたら、通常の lint / unit / build / E2E 判断とは別に、対象画面のスクリーンショット確認を入れる。
- 単発確認の第一候補は `playwright-cli` コマンドとする。
- まずスクショを取り、崩れや視認性の問題があれば修正し、必要なら再度スクショを取り直す。
- このフェーズは見た目確認用であり、E2E の代替ではない。
- 単発の確認のために使い捨ての Playwright spec は作らない。繰り返し検証価値があるものだけ E2E spec にする。
- URL だけで再現できない状態でも、まずは `playwright-cli` の単発操作で足りるかを確認する。一時スクリプトや一時 spec は最後の手段にする。
- 実サイト URL、実作品タイトル、第三者作品由来の本文・raw HTML は画面確認 fixture に使わない。

## 既定コマンド

```bash
playwright-cli open http://127.0.0.1:5173/ --browser=webkit --headed
playwright-cli snapshot
playwright-cli screenshot --filename ui-review-results/library-top-iphone-16e.png
```

- panel、modal、popover、展開状態など、事前操作が必要な確認はこれを第一候補にする。
- 1 回限りの確認なら、まず `open`、`click`、`goto`、`snapshot`、`screenshot` で足りるかを見る。

## まず使う synthetic fixture

- E2E サービスを上げた Dev Container では、合成 fixture の作品を画面確認用の題材として使う。
- 推奨タイトル: `E2E ケースD 本文操作`
- 推奨話: `episode=2`
- 推奨理由:
  - synthetic fixture であり、第三者作品由来の本文を含まない。
  - 本文量が十分あり、reader の下部操作、ページ送り、栞 panel の視認性確認に向いている。

`novelId` は固定値を手書きせず、library API からタイトルで引く。

```bash
export E2E_ORIGIN="http://viewer-web-e2e:15173"
export REVIEW_TITLE="E2E ケースD 本文操作"
export REVIEW_EPISODE="2"

export REVIEW_NOVEL_ID="$(
  python - <<'PY'
import json
import os
import urllib.parse
import urllib.request

origin = os.environ["E2E_ORIGIN"]
title = os.environ["REVIEW_TITLE"]

with urllib.request.urlopen(f"{origin}/api/library/novels") as res:
    data = json.load(res)

for novel in data["novels"]:
    if novel.get("title") == title:
        print(novel["novelId"])
        break
else:
    raise SystemExit(f"synthetic fixture not found: {title}")
PY
)"
```

## 端末 config テンプレ

- `playwright-cli` では `resize` だけだと `isMobile` / `hasTouch` 分岐が揃わないことがある。
- narou-viewer の UI 確認では、まず `--config` で context を寄せる。
- ファイル名はコードブロック外で扱う。
  例: `/tmp/pw-pc.json`、`/tmp/pw-ipad.json`、`/tmp/pw-iphone.json`

```json
{
  "browser": {
    "contextOptions": {
      "viewport": { "width": 1024, "height": 768 },
      "screen": { "width": 1024, "height": 768 },
      "isMobile": false,
      "hasTouch": false,
      "deviceScaleFactor": 1
    }
  }
}
```

```json
{
  "browser": {
    "contextOptions": {
      "viewport": { "width": 768, "height": 1024 },
      "screen": { "width": 768, "height": 1024 },
      "isMobile": true,
      "hasTouch": true,
      "deviceScaleFactor": 2,
      "userAgent": "Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
    }
  }
}
```

```json
{
  "browser": {
    "contextOptions": {
      "viewport": { "width": 390, "height": 844 },
      "screen": { "width": 390, "height": 844 },
      "isMobile": true,
      "hasTouch": true,
      "deviceScaleFactor": 3,
      "userAgent": "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
    }
  }
}
```

- ブラウザ engine の違いを見たい意図がない限り、まず `--browser=webkit` でよい。
- 特に単発の見た目確認では、「PC だけ別 engine」にこだわるより、まず確実に開いて撮ることを優先する。

## 3 パターンの見方

- `pc-xga`: `playwright-cli -s=pc open <url> --browser=webkit --config=/tmp/pw-pc.json`
- `ipad-mini`: `playwright-cli -s=ipad open <url> --browser=webkit --config=/tmp/pw-ipad.json`
- `iphone-16e`: `playwright-cli -s=iphone open <url> --browser=webkit --config=/tmp/pw-iphone.json`
- session 名を固定すると、3 端末を並行で開いて `snapshot` と `screenshot` を順番に回しやすい。

## 栞 panel 用の定型レシピ

- 栞 panel のように、URL だけでは状態が揃わない確認では、まず API で state を作ってから `playwright-cli` に渡す。
- viewer-web の E2E origin を使う場合、まず `bun run e2e:services:up` を実行する。
- 以降の例では、前述の手順で `E2E_ORIGIN`、`REVIEW_NOVEL_ID`、`REVIEW_EPISODE` が設定済みである前提にする。

```bash
curl -s "${E2E_ORIGIN}/api/bookmarks?novelId=${REVIEW_NOVEL_ID}"
```

- 栞を作り直すときは、いったん全削除してから必要件数だけ追加する。
- `bookmarkId` 探索は `jq` がなくても `python - <<'PY'` で済ませてよいが、まずは既存 helper や API の素直な呼び出しを優先する。

```bash
python - <<'PY'
import json
import os
import urllib.request

origin = os.environ["E2E_ORIGIN"]
novel_id = os.environ["REVIEW_NOVEL_ID"]

with urllib.request.urlopen(f"{origin}/api/bookmarks?novelId={novel_id}") as res:
    data = json.load(res)

for bookmark in data["bookmarks"]:
    req = urllib.request.Request(
        f"{origin}/api/bookmarks/{bookmark['id']}",
        method="DELETE",
    )
    urllib.request.urlopen(req).read()
PY

curl -s -X POST "${E2E_ORIGIN}/api/bookmarks" \
  -H 'content-type: application/json' \
  --data '{"novelId":"'"${REVIEW_NOVEL_ID}"'","episodeIndex":2,"position":0}'

curl -s -X POST "${E2E_ORIGIN}/api/bookmarks" \
  -H 'content-type: application/json' \
  --data '{"novelId":"'"${REVIEW_NOVEL_ID}"'","episodeIndex":2,"position":120}'
```

- モバイル reader の栞 panel は、次の URL と操作で再現しやすい。

```bash
playwright-cli -s=ipad open "${E2E_ORIGIN}/?novelId=${REVIEW_NOVEL_ID}&episode=${REVIEW_EPISODE}" --browser=webkit --config=/tmp/pw-ipad.json
playwright-cli -s=ipad snapshot
# snapshot で ref を見てから「栞」を click

playwright-cli -s=iphone open "${E2E_ORIGIN}/?novelId=${REVIEW_NOVEL_ID}&episode=${REVIEW_EPISODE}" --browser=webkit --config=/tmp/pw-iphone.json
playwright-cli -s=iphone snapshot
# iPhone は overflow の「その他の操作」から「栞」に入ることがある
```

- PC は reader の `栞` panel ではなく、トップの詳細側に栞一覧が出ることがある。
- 「3 端末で同じ UI」ではなく、「各端末で実際に使われる栞 UI」を撮る前提で判断する。

## URL だけで再現できない UI

- modal、popover、reader panel のようにクリックが必要な状態は、まず `playwright-cli` の単発操作で確認する。
- その場の画面確認が目的なら、まず `playwright-cli open <url> --browser=webkit --headed` で開き、`snapshot` や `click` を使って必要な状態まで進める。
- 単発操作で十分なときは、一時的な node script や shell script を書かない。
- その確認が継続的な回帰テストとして残す価値を持つと判断できる場合だけ、E2E spec に昇格させる。
- 「今回だけ見るため」の spec は repo に残さない。

## artifact の見方

- 画像は `ui-review-results/` に出る。
- 最終報告では、3 パターンを確認したか、どの path を使ったか、未確認の点があれば何かを短く残す。
- `playwright-cli close-all` で session を閉じて終了する。
