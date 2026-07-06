export type ImageViewerState = {
  src: string;
  originalUrl: string | null;
  title: string | null;
  alt: string | null;
  naturalWidth: number | null;
  naturalHeight: number | null;
};

function normalizeOptionalText(value: string | null | undefined): string | null {
  if (typeof value !== "string") {
    return null;
  }

  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export function extractImageViewerState(image: HTMLImageElement): ImageViewerState | null {
  const src = normalizeOptionalText(image.currentSrc) ?? normalizeOptionalText(image.getAttribute("src"));
  if (!src) {
    return null;
  }

  const anchorCandidate = image.closest("a");
  const anchor = anchorCandidate instanceof HTMLAnchorElement ? anchorCandidate : null;
  const href =
    normalizeOptionalText(image.dataset.readerImageOriginalHref) ??
    normalizeOptionalText(anchor?.dataset.readerImageOriginalHref) ??
    normalizeOptionalText(anchor?.getAttribute("href"));
  const originalUrl = href && !/^javascript:/i.test(href) && !href.startsWith("#") ? href : src;
  const title =
    normalizeOptionalText(image.dataset.readerImageOriginalTitle) ??
    normalizeOptionalText(image.getAttribute("title")) ??
    normalizeOptionalText(anchor?.getAttribute("title")) ??
    normalizeOptionalText(image.getAttribute("alt"));
  const alt = normalizeOptionalText(image.getAttribute("alt"));

  return {
    src,
    originalUrl,
    title,
    alt,
    naturalWidth: image.naturalWidth > 0 ? image.naturalWidth : null,
    naturalHeight: image.naturalHeight > 0 ? image.naturalHeight : null
  };
}

export function calculateImageViewerWidth(
  image: Pick<ImageViewerState, "naturalHeight" | "naturalWidth">,
  zoomPercent: number,
  viewport: { width: number; height: number } = { width: window.innerWidth, height: window.innerHeight }
): number | null {
  if (!image.naturalWidth || !image.naturalHeight) {
    return null;
  }

  const maxViewportWidth = viewport.width * 0.92;
  const maxViewportHeight = viewport.height * 0.78;
  const widthByHeight = (maxViewportHeight * image.naturalWidth) / image.naturalHeight;
  const fittedWidth = Math.min(image.naturalWidth, maxViewportWidth, widthByHeight);

  return Math.max(160, fittedWidth * (zoomPercent / 100));
}
