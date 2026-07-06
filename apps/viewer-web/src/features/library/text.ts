export function normalizeStoryText(value: string): string {
  return decodeHtmlEntities(
    value
      .replace(/<\s*br\s*\/?\s*>/gi, "\n")
      .replace(/<\s*\/\s*p\s*>/gi, "\n")
      .replace(/<[^>]*>/g, "")
  )
    .replace(/[^\S\r\n]+/g, " ")
    .replace(/ *\r?\n */g, "\n")
    .replace(/\n+/g, "\n")
    .trim();
}

function decodeHtmlEntities(value: string): string {
  return value.replace(/&(#x[\da-f]+|#\d+|[a-z]+);/gi, (match, entity: string) => {
    const normalized = entity.toLowerCase();
    if (normalized.startsWith("#x")) {
      const codePoint = Number.parseInt(normalized.slice(2), 16);
      return formatHtmlEntityCodePoint(codePoint, match);
    }
    if (normalized.startsWith("#")) {
      const codePoint = Number.parseInt(normalized.slice(1), 10);
      return formatHtmlEntityCodePoint(codePoint, match);
    }

    switch (normalized) {
      case "amp":
        return "&";
      case "lt":
        return "<";
      case "gt":
        return ">";
      case "quot":
        return '"';
      case "apos":
        return "'";
      case "nbsp":
        return " ";
      default:
        return match;
    }
  });
}

function formatHtmlEntityCodePoint(codePoint: number, fallback: string): string {
  if (!Number.isFinite(codePoint) || codePoint < 0 || codePoint > 0x10ffff) {
    return fallback;
  }
  return String.fromCodePoint(codePoint);
}

export function createStoryPreview(value: string, maxLength: number): { text: string; isTruncated: boolean } {
  const characters = Array.from(value);
  if (characters.length <= maxLength) {
    return {
      text: value,
      isTruncated: false
    };
  }

  return {
    text: `${characters.slice(0, maxLength).join("")}…`,
    isTruncated: true
  };
}
