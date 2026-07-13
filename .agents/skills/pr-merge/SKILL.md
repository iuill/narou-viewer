---
name: pr-merge
description: Use when the user explicitly asks to merge a narou-viewer Pull Request, or asks to finish cleanup after a confirmed merge. Verify merge readiness, use a merge commit, update the local base branch to the remote HEAD, and safely remove merged remote and local work branches.
---

# PR Merge

この skill は、Pull Request の merge と、その後の GitHub / local checkout の状態収束を一続きの作業として扱う。最初に [`AGENTS.md`](../../../AGENTS.md) を読む。

## 安全条件

- ユーザーが明示的に依頼した場合だけ merge を実行する。PR 作成、レビュー、CI 完了だけから merge の許可を推測しない。
- merge commit を使用し、squash merge と rebase merge は選ばない。
- merge 前に PR の repository、番号、base branch、head branch、head repository、head SHA を記録する。
- draft、merge conflict、未完了または失敗中の required check、未対応の requested changes があれば merge しない。
- merge 直前に PR 状態と check / review を再取得する。古い取得結果だけで判断しない。
- 未コミット変更を stash、破棄、別 branch へ持ち越さない。切り替えが必要な worktree が dirty なら後処理を止め、残件を報告する。
- branch を削除する前に `git worktree list --porcelain` で別 worktree の使用有無を確認する。worktree 自体は明示依頼なしに削除しない。

## 1. merge 前確認

1. GitHub App または `gh` で PR metadata、check、review、review thread を確認する。
2. `git status --short --branch`、`git branch -vv`、`git worktree list --porcelain` で local 状態を確認する。
3. PR 本文が最新差分、ユーザー影響、互換性・移行、検証結果と一致していることを確認する。
4. 同一 repository の head branch だけを自動削除対象とする。fork の head branch は勝手に削除しない。

## 2. merge

- merge method と branch 削除を明示できる GitHub App を優先する。CLI を使う場合の第一候補は次とする。

```bash
gh pr merge <number> --merge --delete-branch
```

- コマンドの終了だけで成功と判断せず、PR を再取得して `merged`、merge commit SHA、base / head、merged time を確認する。
- 実際の merge commit が複数 parent を持つことを確認する。単一 parent なら squash または rebase 相当として報告し、後処理では下記の強制削除条件を適用する。

## 3. base branch を最新化

base branch が現在の clean worktree で切り替え可能な場合は、次を実行する。

```bash
git fetch --prune origin
git switch <base-branch>
git pull --ff-only origin <base-branch>
```

- base branch が別 worktree で checkout 済みなら、その worktree が clean な場合だけそこで `git pull --ff-only` する。dirty または利用中なら勝手に変更せず報告する。
- `git rev-parse <base-branch>` と `git rev-parse origin/<base-branch>` が一致することを確認する。
- merge commit SHA が `origin/<base-branch>` の ancestor であることを確認する。

## 4. branch cleanup

- remote head branch は `git ls-remote --heads origin refs/heads/<head-branch>` で確認する。残っている場合、同一 repository かつ remote tip が記録済み PR head SHA と一致するときだけ削除する。進んでいる branch は削除しない。
- fetch 後に stale な remote-tracking ref が消えたことを確認する。
- local head branch は別 worktree で未使用であり、local tip が記録済み PR head SHA と一致することを確認してから、まず次を実行する。

```bash
git branch -d <head-branch>
```

- squash / rebase 済みの既存 PR では `-d` が失敗し得る。PR が merged、merge commit が最新 `origin/<base-branch>` の ancestor、local tip が記録済み PR head SHA と一致する、別 worktree で未使用、という全条件を満たす場合だけ `git branch -D <head-branch>` を許可する。
- SHA 不一致、dirty worktree、別 worktree での使用、権限不足などで削除できない場合は、その状態を残して理由を報告する。

## 5. 完了報告

次をまとめて報告する。

- merged PR 番号と URL
- merge method と merge commit SHA
- base branch の local / remote HEAD 一致
- remote / remote-tracking / local head branch の削除結果
- 安全上残した branch や worktree と、その理由
