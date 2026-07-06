export type RouteSelection = {
  novelId: string | null;
  episodeIndex: string | null;
  position: number | null;
  screenMode: "library" | "reader";
};

export function parsePosition(value: string | null): number | null {
  if (typeof value !== "string" || !/^\d+$/.test(value)) {
    return null;
  }

  return Number.parseInt(value, 10);
}

export function parseRouteSelection(search: string = window.location.search): RouteSelection {
  const params = new URLSearchParams(search);
  const novelId = params.get("novelId");
  const episodeParam = params.get("episode");
  const positionParam = params.get("pos");
  const episodeIndex = typeof episodeParam === "string" && /^\d+$/.test(episodeParam) ? episodeParam : null;
  const position = parsePosition(positionParam);

  return {
    novelId,
    episodeIndex,
    position,
    screenMode: novelId && episodeIndex !== null ? "reader" : "library"
  };
}
