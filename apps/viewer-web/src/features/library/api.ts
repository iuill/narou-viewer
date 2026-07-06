import { requestJson } from "../../api/http";
import { fetchRuntimeStatus } from "../runtime/api";
import type { InitialData, NovelsResponse } from "./types";
import type { RuntimeStatusResponse } from "../runtime/types";

export async function fetchInitialData(): Promise<InitialData> {
  let runtimeStatus: RuntimeStatusResponse;
  let novelsResponse: NovelsResponse;
  try {
    [runtimeStatus, novelsResponse] = await Promise.all([
      fetchRuntimeStatus(),
      requestJson<NovelsResponse>("/api/library/novels", undefined, "初期データの取得に失敗しました。")
    ]);
  } catch {
    throw new Error("初期データの取得に失敗しました。");
  }

  return {
    runtimeStatus,
    novels: novelsResponse.novels
  };
}
