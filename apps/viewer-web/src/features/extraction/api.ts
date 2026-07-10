import { mutateJson, requestJson } from "../../api/http";
import type {
  ExtractionClearResponse,
  ExtractionJobsResponse,
  ExtractionSubmitRequest,
  ExtractionSubmitResponse,
} from "./types";

export async function clearExtraction(
  novelId: string,
): Promise<ExtractionClearResponse> {
  return requestJson<ExtractionClearResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/extraction`,
    { method: "DELETE" },
    "抽出データのクリアに失敗しました。",
  );
}

export async function fetchExtractionJobs(
  novelId: string,
): Promise<ExtractionJobsResponse> {
  return requestJson<ExtractionJobsResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/extraction-jobs`,
    undefined,
    "抽出履歴の取得に失敗しました。",
  );
}

export async function submitExtraction(
  novelId: string,
  payload: ExtractionSubmitRequest,
): Promise<ExtractionSubmitResponse> {
  return mutateJson<ExtractionSubmitResponse, ExtractionSubmitRequest>(
    `/api/library/novels/${encodeURIComponent(novelId)}/extraction-jobs`,
    payload,
    "人物と用語の抽出依頼に失敗しました。",
  );
}
