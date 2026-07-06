function normalizeOptionalText(value: string | null | undefined): string | null {
  if (typeof value !== "string") {
    return null;
  }

  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export function extractDroppedDownloadTarget(dataTransfer: Pick<DataTransfer, "getData"> | null): string | null {
  if (!dataTransfer) {
    return null;
  }

  const uriList = normalizeOptionalText(dataTransfer.getData("text/uri-list"));
  if (uriList) {
    const firstUri = uriList
      .split(/\r?\n/)
      .map((entry) => entry.trim())
      .find((entry) => entry.length > 0 && !entry.startsWith("#"));

    if (firstUri) {
      return firstUri;
    }
  }

  const plainText = normalizeOptionalText(dataTransfer.getData("text/plain"));
  if (!plainText) {
    return null;
  }

  return (
    plainText
      .split(/\r?\n/)
      .map((entry) => entry.trim())
      .find((entry) => entry.length > 0) ?? null
  );
}
