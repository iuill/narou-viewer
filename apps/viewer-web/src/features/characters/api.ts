import { requestJson } from "../../api/http";
import type { EpisodeIndex } from "../reader/types";
import type { CharacterSummaryResponse } from "./types";

export async function fetchCharacterSummary(novelId: string, upToEpisodeIndex: EpisodeIndex): Promise<CharacterSummaryResponse> {
  return requestJson<CharacterSummaryResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/characters?upToEpisodeIndex=${encodeURIComponent(upToEpisodeIndex)}`,
    undefined,
    "キャラクター一覧の取得に失敗しました。"
  );
}
