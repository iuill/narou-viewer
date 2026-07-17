# novel-fetcher task recovery runbook

この文書は Issue #15 で追加した `novel-fetcher` の永続 task queue を、再起動・backup restore・一時停止後に確認するための運用手順です。

## 状態の意味

- `queued`: queue 順に自動実行される。
- `running`: 現在実行中。process 再起動後に残っていた場合は startup recovery の対象になる。
- `paused`: 利用者が一時停止した task。自動実行せず、同じ task ID の再開操作を待つ。
- `interrupted`: shutdown、異常終了、または recovery により実行が確定しなかった task。自動再実行せず、明示的な再開を待つ。
- `failed`: 処理エラーで終了した task。原因を確認してから再試行する。
- `canceled`: 利用者が破棄した task。task-level resume は行わず、必要なら新しい取得要求を投入する。
- `succeeded`: 保存処理の commit fence まで完了した task。commit 後に遅れて cancel が記録された場合も、起動時 recovery は確定済みの保存結果を優先する。

`running` の `requestedAction` が `pause` または `cancel` の間は、実行中の HTTP request、retry wait、host rate-limit wait が停止するのを待ちます。
永続化済みの同じ action の再送は `changed:false` として冪等に扱い、異なる action への切り替えは `409 Conflict` とするため、最初の action が terminal state へ確定してから次の操作を行います。
永続化が一時的に失敗した場合は、停止済み context を元に戻さず、同じ action の再送と runner の finalization が SQLite 書き込みを再試行します。
UI は optimistic に最終状態を確定せず、次の polling 結果を正本として扱います。

download の予約 identity は、対応サイトの作品 ID を正規形とします。同じ作品を N コードと小説家になろう URL、またはカクヨムの作品 URL と episode URL で重ねて投入しても同一作品として扱います。同じ identity・option の要求は既存 task ID へ dedupe し、option が異なる要求や同じ作品への update / resume は先行 task が予約を解放するまで conflict にします。

## 通常の確認

1. viewer の取得状況で、現在の task、待機列、一時停止・中断件数を確認する。
2. task card の進捗、対象、`resumeEpisodeId` を確認する。
3. `paused` / `interrupted` / `failed` は、原因と保存済み話数を確認してから「再開」または「再試行」を押す。
4. 再開後は同じ task ID のまま queue 末尾へ入り、同じ task の有効な episode checkpoint は再取得されないことを確認する。

内部 API を直接確認する場合は、取得 sidecar の task API ではなく、通常の BFF 経路を使います。

```text
GET  /api/fetcher/tasks/summary
POST /api/fetcher/tasks/{taskId}/pause
POST /api/fetcher/tasks/{taskId}/resume
POST /api/fetcher/tasks/{taskId}/cancel
```

## 再起動・restore 後

1. writer を停止した状態で cold backup / restore を実施する。
2. 対応 build を起動し、migration と startup recovery のログを確認する。
3. `queued` が queue 順に実行されることを確認する。
4. `interrupted` が勝手に実行されていないことを確認する。必要なものだけ明示的に再開する。
5. state doctor を実行し、`NF-LIBRARY` の schema、SQLite integrity、canonical file の error finding がないことを確認する。task request・queue・checkpoint の invariant は novel-fetcher の startup recovery のログで確認する。

```bash
bun run state:doctor --data-dir ./data
```

`queued` task に queue row がない、queued 以外に queue row がある、再開可能な task の未知 request version、同一作品の予約 task 重複などは自動推測で修復しません。
対応 build または同じ consistency group の supported backup を使って復旧してください。
startup recovery が request を検証するのは `queued`、`running`、`paused`、`interrupted`、`failed` です。
再開しない `succeeded` / `canceled` の履歴は起動可否を左右させませんが、保持期間は別途運用ポリシーを定めるまで自動削除しません。

task state の読み取りに失敗した場合、sidecar は空キューとして成功応答を返しません。HTTP 5xx と `failed to read task ... state` のログを storage 障害として扱い、SQLite と canonical file の診断を行ってください。

## 一時停止・中止の判断

- 一時停止: 保存済み episode を保持して、後で同じ task を続行したい場合。
- 中止: task を破棄し、同じ対象を新しい option で投入し直す場合。
- 中断: process の終了や障害で実行が確定しなかった場合。resume 前に外部副作用との重複が許容できるかを確認する。

runner は attempt ごとに `starting`、`running`、`finalizing` の phase を持ちます。control と claim handoff は task ID 単位で直列化し、`ClaimNext` 後の `requested_action` 初期同期が完了してから executor を開始します。これにより、開始時の handoff で SQLite が受理した action と異なる context cause が選ばれることはありません。別 task の control は同じ lock を共有しないため、SQLite writer を待っている queued task が実行中 task の context cancellation を妨げることもありません。

`running` 中は、最初の pause または cancel が context を停止してから同じ action を SQLite へ保存します。
この一連の操作は同じ task の範囲で runner が直列化します。
永続化済みの同じ action の再送は `changed:false`、異なる action は `409 Conflict` です。
SQLite 書き込みが一時的に失敗した場合、同じ action の再送は未完了の永続化を再試行し、executor 終了後も runner が backoff 付きで再試行してから terminal state を確定します。
in-memory queue を使うテスト構成でも同じ先着規則を適用します。

executor が終了すると、task は SQLite の terminal 更新が完了して runner が attempt を解放するまで `finalizing` になります。
この間の新しい pause、cancel、resume は `409 Conflict` であり、`changed:true` として受理しません。
既に受理済みの同じ pause または cancel を並行して再送した場合だけ、最初の結果を `changed:false` として返します。
resume は runner が旧 attempt を解放した後に再送してください。
再開後に同じ resume を再送し、task がすでに `queued` または `running` なら `changed:false` で成功します。

terminal status の正本は SQLite です。
`Finalize` は同じ transaction 内で `requested_action` と `execution_committed` を読み、未 commit なら durable action を、commit 済みなら `succeeded` を優先します。
作品の完了 commit も `requested_action` が空の場合だけ成功するため、control と完了処理は SQLite 上で先に commit した側が勝ちます。
複数作品を持つ task は各作品を個別に完了へ更新しますが、`execution_committed` を立てるのは最後の作品だけです。
このため、途中の作品完了後に後続作品が失敗しても task 全体の失敗は成功へ上書きされません。

SQLite は単一 writer connection を維持し、task summary、counts、detail は WAL の read-only pool から一貫した snapshot として読みます。
長い作品保存 transaction 中も UI polling は writer connection の解放を待ちません。
control の書き込みは利用者意図を HTTP client の切断で失わないよう runner 管理の context で継続します。

第三者作品の本文、raw HTML、画像、provider の出力をログや issue に貼り付けないでください。障害報告には task ID、status、attempt、phase、進捗、エラー種別だけを記録します。
