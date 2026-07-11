import { requestJson } from "../../api/http";
import type { EpisodeIndex } from "../reader/types";
import type { TermsResponse } from "./types";

export async function fetchTerms(
  novelId: string,
  upToEpisodeIndex: EpisodeIndex,
): Promise<TermsResponse> {
  return requestJson<TermsResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/terms?upToEpisodeIndex=${encodeURIComponent(upToEpisodeIndex)}`,
    undefined,
    "用語一覧の取得に失敗しました。",
  );
}
