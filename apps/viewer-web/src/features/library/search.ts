function normalizeSearchText(value: string): string {
  return value.normalize("NFKC").toLocaleLowerCase("ja-JP").replace(/\s+/g, " ").trim();
}

export function filterNovelsByQuery<
  TNovel extends { title: string; author: string; siteName: string; tocUrl: string | null }
>(novels: TNovel[], query: string): TNovel[] {
  const keywords = normalizeSearchText(query)
    .split(" ")
    .filter((keyword) => keyword.length > 0);

  if (keywords.length === 0) {
    return novels;
  }

  return novels.filter((novel) => {
    const haystack = normalizeSearchText([novel.title, novel.author, novel.siteName, novel.tocUrl ?? ""].join(" "));
    return keywords.every((keyword) => haystack.includes(keyword));
  });
}
