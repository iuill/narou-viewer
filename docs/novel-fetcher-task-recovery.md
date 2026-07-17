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

`running` の `requestedAction` が `pause` または `cancel` の間は、実行中の HTTP request・retry wait・host rate-limit wait が停止するのを待ちます。同じ action の再送は冪等に扱い、異なる action への切り替えは `409 Conflict` とするため、最初の action が terminal state へ確定してから次の操作を行います。UI は optimistic に最終状態を確定せず、次の polling 結果を正本として扱います。

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

`queued` task に queue row がない、queued 以外に queue row がある、未知の request version、同一作品の予約 task 重複などは自動推測で修復しません。対応 build または同じ consistency group の supported backup を使って復旧してください。

task state の読み取りに失敗した場合、sidecar は空キューとして成功応答を返しません。HTTP 5xx と `failed to read task ... state` のログを storage 障害として扱い、SQLite と canonical file の診断を行ってください。

## 一時停止・中止の判断

- 一時停止: 保存済み episode を保持して、後で同じ task を続行したい場合。
- 中止: task を破棄し、同じ対象を新しい option で投入し直す場合。
- 中断: process の終了や障害で実行が確定しなかった場合。resume 前に外部副作用との重複が許容できるかを確認する。

第三者作品の本文、raw HTML、画像、provider の出力をログや issue に貼り付けないでください。障害報告には task ID、status、attempt、phase、進捗、エラー種別だけを記録します。
