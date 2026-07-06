import { requestJson } from "../../api/http";
import type { RuntimeStatusResponse } from "./types";

export async function fetchRuntimeStatus(): Promise<RuntimeStatusResponse> {
  return requestJson<RuntimeStatusResponse>("/api/system/status", undefined, "動作状況の取得に失敗しました。");
}
