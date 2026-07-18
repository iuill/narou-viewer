---
name: pr-merge
description: Use when the user explicitly asks to merge a narou-viewer Pull Request, or asks to finish cleanup after a confirmed merge. Verify merge readiness, use squash merge, update the local base branch to the remote HEAD, and safely remove merged remote and local work branches.
---

# PR Merge

この skill は、Pull Request の merge と、その後の GitHub / local checkout の状態収束を一続きの作業として扱う。最初に [`AGENTS.md`](../../../AGENTS.md) を読む。

## 安全条件

- ユーザーが明示的に依頼した場合だけ merge を実行する。PR 作成、レビュー、CI 完了だけから merge の許可を推測しない。
- squash merge を使用し、merge commit と rebase merge は選ばない。
- GitHub repository settings で squash merge が許可されていなければ停止する。merge commit または rebase merge が許可されている場合は、repository rule と settings の不一致として報告する。
- merge 前に PR URL、base / head repository、base / head branch、head SHA を記録する。
- draft、merge conflict、未完了または失敗中の required check、未対応の requested changes があれば merge しない。
- merge 前に [`docs/quality-goals.md`](../../../docs/quality-goals.md) の変更に関係する項目を確認し、変更内容と採用した品質対策が適合していることを確認する。
- merge 直前に PR 状態と check / review を再取得する。古い取得結果だけで判断しない。
- `git remote get-url --all` で各 remote の fetch URL を列挙し、PR の base repository と一意に対応する `base_remote` と `validated_base_fetch_url` を決める。決められなければ base 同期を行わず報告する。
- 同一 repository の head branch を削除するときだけ、`git remote get-url --push --all` の結果が単一で head repository と一致する `head_remote` と `validated_head_push_url` を要求する。fork では head remote 未設定を正常として cleanup を省略する。
- 未コミット変更を stash、破棄、別 branch へ持ち越さない。切り替えが必要な worktree が dirty なら後処理を止め、残件を報告する。
- branch を削除する前に `git worktree list --porcelain` で別 worktree の使用有無を確認する。worktree 自体は明示依頼なしに削除しない。

## 1. merge 前確認

1. GitHub App または `gh` で PR metadata、check、review、review thread を確認する。
2. `gh repo view --json squashMergeAllowed,mergeCommitAllowed,rebaseMergeAllowed` で merge method の repository settings を確認する。
3. `git status --short --branch`、`git branch -vv`、`git worktree list --porcelain` で local 状態を確認する。
4. 変更に関係する `docs/quality-goals.md` の項目に適合していることを確認する。目標と異なる判断が必要な場合は、その理由と影響が関連仕様と PR 本文に明記されるまで merge しない。
5. PR 本文が最新差分、ユーザー影響、互換性・移行、検証結果と一致していることを確認し、差異があれば更新する。
6. PR から参照または close される関連 issue を確認する。完了条件、採用した判断、残課題を実装と照合し、変更点は原則として issue コメントに記録する。完了条件自体を正式に変更する場合だけ issue 本文を更新する。
7. 同一 repository の head branch だけを自動削除対象とする。fork の head branch は勝手に削除しない。
8. 削除直前に同じ head repository / branch を使う対象外の open PR を再検索する。1件でもある、または完全に確認できない場合は remote / local branch を削除しない。
9. branch の自動 cleanup は現在のエージェント作業で作成した branch に限定する。それ以外は、ユーザーがその branch 名を指定して削除を許可した場合だけ削除する。

## 2. merge

- 記録済み head SHA との一致を merge 条件にでき、head branch を残せる GitHub App を使用してよい。それ以外では次の CLI を使う。

```bash
gh pr merge "$pr_url" --squash --match-head-commit "$head_sha"
```

- merge と branch 削除を同じ操作で行わない。head SHA 不一致なら PR 状態を再取得し、merge せず停止する。
- コマンドの終了だけで成功と判断せず、PR を再取得して `merged`、squash commit SHA（GitHub API では `mergeCommitSha` / `merge_commit_sha`）、base / head、merged time を確認する。
- squash commit が単一 parent を持ち、最新の base branch の ancestor であることを確認する。複数 parent なら merge commit 相当として報告し、後処理を止める。

## 3. base branch を最新化

base branch が現在の clean worktree で切り替え可能な場合は、次を実行する。

```bash
git fetch --no-prune --no-tags "$validated_base_fetch_url" \
  "+refs/heads/$base_branch:refs/remotes/$base_remote/$base_branch"
git switch "$base_branch"
git merge --ff-only "refs/remotes/$base_remote/$base_branch"
```

- base branch が別 worktree で checkout 済みなら、その worktree が clean な場合だけそこで `git merge --ff-only` する。dirty または利用中なら勝手に変更せず報告する。
- `git rev-parse "refs/heads/$base_branch"` と `git rev-parse "refs/remotes/$base_remote/$base_branch"` が一致することを確認する。
- `git merge-base --is-ancestor "$squash_commit_sha" "refs/remotes/$base_remote/$base_branch"` が成功することを確認する。

## 4. branch cleanup

- remote head branch は同一 repository の PR だけを削除対象とし、確認と削除を分離せず次の lease 付き push で処理する。remote tip が記録済み PR head SHA から進んでいれば削除は拒否される。

```bash
git push \
  --force-with-lease="refs/heads/$head_branch:$head_sha" \
  "$validated_head_push_url" \
  ":refs/heads/$head_branch"
```

- lease 付き削除が拒否された場合は通常の `--delete` へ切り替えない。remote branch が既にないか、tip が進んでいるかを読み取りで確認し、残件を報告する。
- lease 付き削除後は対象の remote-tracking ref だけを期待 SHA 付きで削除する。SHA が異なる場合は削除せず報告する。

```bash
git update-ref -d \
  "refs/remotes/$head_remote/$head_branch" \
  "$head_sha"
```

- local head branch は別 worktree で未使用であり、local tip が記録済み PR head SHA と一致することを確認する。squash 後の head commit は base branch の ancestor にならないため、通常の `git branch -d` は使わない。

```bash
git branch -D "$head_branch"
```

- `git branch -D` は、PR が squash merge 済み、squash commit が最新 `refs/remotes/$base_remote/$base_branch` の ancestor、`refs/heads/$head_branch` が記録済み PR head SHA と一致する、別 worktree で未使用、という全条件を満たす場合だけ許可する。
- SHA 不一致、dirty worktree、別 worktree での使用、権限不足などで削除できない場合は、その状態を残して理由を報告する。

## 5. 完了報告

次をまとめて報告する。

- merged PR 番号と URL
- merge method と squash commit SHA
- 品質目標への適合確認と、更新した PR 本文、関連 issue へ記録した変更点
- base branch の local / remote HEAD 一致
- remote / remote-tracking / local head branch の削除結果
- 安全上残した branch や worktree と、その理由
