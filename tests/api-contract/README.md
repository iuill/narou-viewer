# API contract tests

このディレクトリは、`viewer-api` の HTTP API 外部契約を固定するための black-box test suite です。
テストは `API_BASE_URL` にだけ依存し、Go の内部 package を import しません。

```bash
API_BASE_URL=http://viewer-api-e2e:18080 bun run test:api-contract
```

状態を書き換える contract を実行する場合は、fixture 専用環境に対して次のように実行します。

```bash
API_BASE_URL=http://viewer-api-e2e:18080 API_CONTRACT_MUTATING=1 bun run test:api-contract
```

CI や Phase 1 gate では fixture が空の場合に落とすため、次の strict mode を使います。

```bash
API_BASE_URL=http://viewer-api-e2e:18080 API_CONTRACT_MUTATING=1 API_CONTRACT_REQUIRE_FIXTURE=1 bun run test:api-contract
```

取得 backend の update / cancel / remove と remove 後 cleanup を確認する destructive contract は、
通常の mutating contract と分けて明示的に有効化します。誤って通常の dev / shared data を削らないよう、
`API_BASE_URL` は `viewer-api-e2e` または `localhost:18080` / `127.0.0.1:18080` だけを許可し、
削除対象も `API_CONTRACT_DESTRUCTIVE_FETCHER_TARGET_NOVEL_ID` で明示します。

```bash
API_BASE_URL=http://viewer-api-e2e:18080 API_CONTRACT_MUTATING=1 API_CONTRACT_REQUIRE_FIXTURE=1 API_CONTRACT_DESTRUCTIVE_FETCHER=1 API_CONTRACT_DESTRUCTIVE_FETCHER_TARGET_NOVEL_ID=c2l0ZTpzeW9zZXR1Om4xMjM0YWI node ./node_modules/vitest/vitest.mjs run --config tests/api-contract/vitest.config.ts tests/api-contract/cases/zz-fetcher-mutating.test.ts
```

通常は先に E2E service を起動します。

```bash
bun run e2e:services:up
API_BASE_URL=http://viewer-api-e2e:18080 bun run test:api-contract
```

Dev Container 内から service name で直接叩く場合は、上記の `API_BASE_URL=http://viewer-api-e2e:18080`
を使えます。host runner から叩く場合は、公開 port の `API_BASE_URL=http://127.0.0.1:18080` を使います。

## 方針

- Go 実装にも同じ suite を流せるよう、HTTP response のみを見る。
- `API_BASE_URL` は必須。未指定時の既定値は置かず、対象 API の取り違えを避ける。
- timestamp や環境依存の service 状態は型と必須 field を確認し、値の完全一致は避ける。
- mutation を伴うテストは `API_CONTRACT_MUTATING=1` で明示的に有効化する。
- fetcher の destructive mutation を伴うテストは `API_CONTRACT_DESTRUCTIVE_FETCHER=1` と `API_CONTRACT_DESTRUCTIVE_FETCHER_TARGET_NOVEL_ID` でさらに明示的に有効化する。
- fixture 必須 gate は `API_CONTRACT_REQUIRE_FIXTURE=1` で明示的に有効化する。
- snapshot は初期段階では使わず、required fields / nullable fields / error shape を matcher で固定する。
