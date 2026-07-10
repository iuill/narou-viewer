import { mutateJson, requestJson } from "../../api/http";
import type { EpisodeIndex } from "../reader/types";
import type {
  CharacterJobSubmitRequest,
  CharacterJobSubmitResponse,
  CharacterJobsResponse,
  CharacterSummaryClearResponse,
  CharacterSummaryResponse
} from "./types";

export async function fetchCharacterSummary(novelId: string, upToEpisodeIndex: EpisodeIndex): Promise<CharacterSummaryResponse> {
  return requestJson<CharacterSummaryResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/characters?upToEpisodeIndex=${encodeURIComponent(upToEpisodeIndex)}`,
    undefined,
    "キャラクター一覧の取得に失敗しました。"
  );
}

export async function clearCharacterSummary(novelId: string): Promise<CharacterSummaryClearResponse> {
  return requestJson<CharacterSummaryClearResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/extraction`,
    { method: "DELETE" },
    "キャラクター一覧生成データのクリアに失敗しました。"
  );
}

export async function fetchCharacterJobs(novelId: string): Promise<CharacterJobsResponse> {
  return requestJson<CharacterJobsResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/extraction-jobs`,
    undefined,
    "キャラクター生成履歴の取得に失敗しました。"
  );
}

export async function submitCharacterJob(novelId: string, payload: CharacterJobSubmitRequest): Promise<CharacterJobSubmitResponse> {
  return mutateJson<CharacterJobSubmitResponse, CharacterJobSubmitRequest>(
    `/api/library/novels/${encodeURIComponent(novelId)}/extraction-jobs`,
    payload,
    "キャラクター一覧生成の依頼に失敗しました。"
  );
}
