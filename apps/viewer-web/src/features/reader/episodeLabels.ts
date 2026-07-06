export type EpisodeLike = {
  episodeIndex: string;
  title: string;
  chapter: string | null;
  subchapter: string | null;
};

export type EpisodeDisplayReference<TEpisode extends EpisodeLike = EpisodeLike> = TEpisode & {
  order: number;
};

export function buildEpisodeLabel(episode: Pick<EpisodeLike, "chapter" | "subchapter" | "title">): string {
  const chapter = [episode.chapter, episode.subchapter].filter(Boolean).join(" / ");
  return chapter.length > 0 ? `${chapter} - ${episode.title}` : episode.title;
}

export function buildEpisodeDisplayLookup<TEpisode extends EpisodeLike>(
  episodes: TEpisode[]
): Map<string, EpisodeDisplayReference<TEpisode>> {
  return new Map(episodes.map((episode, index) => [episode.episodeIndex, { ...episode, order: index + 1 }]));
}

export function shouldUseFriendlyEpisodeLabels(siteName: string | null | undefined, tocUrl: string | null | undefined): boolean {
  return (
    (typeof siteName === "string" && siteName.includes("カクヨム")) ||
    (typeof tocUrl === "string" && tocUrl.includes("kakuyomu.jp"))
  );
}

export function formatEpisodeIndexLabel<TEpisode extends EpisodeLike>(
  episodeIndex: string,
  episodeLookup: Map<string, EpisodeDisplayReference<TEpisode>>,
  preferFriendlyEpisodeLabels: boolean
): string {
  const episode = episodeLookup.get(episodeIndex);
  if (preferFriendlyEpisodeLabels && episode) {
    return `#${episode.order}`;
  }

  return `#${episodeIndex}`;
}

export function formatEpisodeOrderLabel<TEpisode extends EpisodeLike>(
  episodeIndex: string,
  episodeLookup: Map<string, EpisodeDisplayReference<TEpisode>>
): string {
  const episode = episodeLookup.get(episodeIndex);
  return episode ? String(episode.order) : episodeIndex;
}

export function formatEpisodeReferenceLabel<TEpisode extends EpisodeLike>(
  episodeIndex: string,
  episodeLookup: Map<string, EpisodeDisplayReference<TEpisode>>,
  preferFriendlyEpisodeLabels: boolean
): string {
  const episode = episodeLookup.get(episodeIndex);
  if (preferFriendlyEpisodeLabels && episode) {
    return episode.title;
  }

  return `#${episodeIndex}`;
}
