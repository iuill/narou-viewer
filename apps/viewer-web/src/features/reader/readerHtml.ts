function normalizeOptionalText(value: string | null | undefined): string | null {
  if (typeof value !== "string") {
    return null;
  }

  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export function prepareEpisodeHtmlForReader(
  source: string,
  doc: Document | undefined = typeof document === "undefined" ? undefined : document
): string {
  if (source.length === 0 || !doc) {
    return source;
  }

  const template = doc.createElement("template");
  template.innerHTML = source;

  const imageAnchors = Array.from(template.content.querySelectorAll<HTMLAnchorElement>("a[href]"));
  for (const anchor of imageAnchors) {
    if (anchor.childElementCount !== 1 || (anchor.textContent?.trim() ?? "").length > 0) {
      continue;
    }

    const image = anchor.firstElementChild;
    if (!(image instanceof HTMLImageElement)) {
      continue;
    }

    image.dataset.readerImageOriginalHref = anchor.href;
    const anchorTitle = normalizeOptionalText(anchor.getAttribute("title"));
    if (anchorTitle) {
      image.dataset.readerImageOriginalTitle = anchorTitle;
    }

    anchor.replaceWith(image);
  }

  return template.innerHTML;
}
