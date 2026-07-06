import { expect } from "vitest";
import {
  expectJsonResponse,
  expectNonEmptyFixtureArray,
  REQUIRE_CONTRACT_FIXTURE,
  requestJson,
} from "./apiClient";

export type FixtureEpisode = {
  novelId: string;
  episodeIndex: string;
};

type FixtureCandidate = FixtureEpisode & {
  novelId: string;
};

export async function findFixtureEpisode(
  label: string,
): Promise<FixtureEpisode | null> {
  const preferredNovelId = process.env.API_CONTRACT_FIXTURE_NOVEL_ID?.trim();
  const preferredEpisodeIndex =
    process.env.API_CONTRACT_FIXTURE_EPISODE_INDEX?.trim();
  const novelsResponse = await requestJson<{
    novels: Array<{ novelId: string; totalEpisodes: number }>;
  }>("/api/library/novels");
  expectJsonResponse(novelsResponse);

  const novelsWithEpisodes = novelsResponse.json.novels.filter(
    (novel) => novel.totalEpisodes > 0,
  );
  expectNonEmptyFixtureArray(
    novelsWithEpisodes,
    `${label} novels with episodes`,
  );

  const sortedNovels = [...novelsWithEpisodes].sort((left, right) =>
    left.novelId.localeCompare(right.novelId),
  );
  const selectedNovel = preferredNovelId
    ? sortedNovels.find((candidate) => candidate.novelId === preferredNovelId)
    : sortedNovels[0];
  if (!selectedNovel) {
    if (preferredNovelId) {
      expect(
        selectedNovel,
        `${label} preferred fixture novel was not found: ${preferredNovelId}`,
      ).toBeDefined();
    }
    return null;
  }

  expect(selectedNovel.novelId).toEqual(expect.any(String));

  const selectedCandidate = await findFixtureEpisodeCandidate(
    label,
    selectedNovel.novelId,
    preferredEpisodeIndex,
  );
  if (!selectedCandidate) {
    return null;
  }

  if (!preferredNovelId && REQUIRE_CONTRACT_FIXTURE) {
    const readyCandidate = await findReadyCharacterFixtureCandidate(
      label,
      sortedNovels.map((novel) => novel.novelId),
      preferredEpisodeIndex,
    );
    if (readyCandidate) {
      return readyCandidate;
    }
  }

  return {
    novelId: selectedCandidate.novelId,
    episodeIndex: selectedCandidate.episodeIndex,
  };
}

export async function findFixtureEpisodeForNovelID(
  label: string,
  novelId: string,
): Promise<FixtureEpisode | null> {
  const trimmedNovelID = novelId.trim();
  expect(trimmedNovelID, `${label} target novelId is required`).not.toBe("");
  return findFixtureEpisodeCandidate(label, trimmedNovelID, undefined);
}

async function findFixtureEpisodeCandidate(
  label: string,
  novelId: string,
  preferredEpisodeIndex: string | undefined,
): Promise<FixtureCandidate | null> {
  const tocResponse = await requestJson<{
    episodes: Array<{ episodeIndex: string }>;
  }>(`/api/library/novels/${encodeURIComponent(novelId)}/toc`);
  expectJsonResponse(tocResponse);
  expectNonEmptyFixtureArray(
    tocResponse.json.episodes,
    `${label} toc episodes`,
  );

  const sortedEpisodes = [...tocResponse.json.episodes].sort((left, right) =>
    left.episodeIndex.localeCompare(right.episodeIndex, undefined, {
      numeric: true,
    }),
  );
  const episode = preferredEpisodeIndex
    ? sortedEpisodes.find(
        (candidate) => candidate.episodeIndex === preferredEpisodeIndex,
      )
    : sortedEpisodes[0];
  if (!episode) {
    if (preferredEpisodeIndex) {
      expect(
        episode,
        `${label} preferred fixture episode was not found: ${preferredEpisodeIndex}`,
      ).toBeDefined();
    }
    return null;
  }

  expect(episode.episodeIndex).toEqual(expect.any(String));
  const candidate = {
    novelId,
    episodeIndex: episode.episodeIndex,
  };
  if (!(await canReadFixtureEpisode(candidate))) {
    return null;
  }
  return candidate;
}

async function canReadFixtureEpisode(candidate: FixtureCandidate): Promise<boolean> {
  const response = await requestJson(
    `/api/library/novels/${encodeURIComponent(candidate.novelId)}/episodes/${encodeURIComponent(candidate.episodeIndex)}`,
  );
  return response.status === 200;
}

async function findReadyCharacterFixtureCandidate(
  label: string,
  novelIds: string[],
  preferredEpisodeIndex: string | undefined,
): Promise<FixtureCandidate | null> {
  for (const novelId of novelIds) {
    const candidate = await findFixtureEpisodeCandidate(
      label,
      novelId,
      preferredEpisodeIndex,
    );
    if (!candidate) {
      continue;
    }

    const response = await requestJson<{ status?: string }>(
      `/api/library/novels/${encodeURIComponent(candidate.novelId)}/characters?upToEpisodeIndex=${encodeURIComponent(
        candidate.episodeIndex,
      )}`,
    );
    if (response.status === 200 && response.json.status === "ready") {
      return candidate;
    }
  }

  return null;
}
