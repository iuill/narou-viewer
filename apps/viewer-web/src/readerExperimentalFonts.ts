export type ReaderExperimentalFontFamilyKind = "既定" | "明朝系" | "ゴシック系";
export type ReaderExperimentalFontSourceLabel = "既定" | "Google Fonts" | "Apple ローカル";
export type ReaderExperimentalFontWeight = 300 | 400 | 500;
export const DEFAULT_READER_EXPERIMENTAL_FONT_ID = "none";

export const READER_EXPERIMENTAL_FONT_WEIGHT_OPTIONS = [
  {
    value: 300,
    label: "Light",
    description: "細めで軽い見え方です。"
  },
  {
    value: 400,
    label: "Regular",
    description: "標準の太さです。"
  },
  {
    value: 500,
    label: "Medium",
    description: "少し太めで視認性を優先します。"
  }
] as const;

export const READER_EXPERIMENTAL_FONT_OPTIONS = [
  {
    id: DEFAULT_READER_EXPERIMENTAL_FONT_ID,
    label: "標準設定を使う",
    description: "読書設定パネルの明朝 / ゴシックをそのまま使います。",
    familyKind: "既定",
    sourceLabel: "既定",
    familyCss: null,
    stylesheetHref: null,
    supportedWeights: null
  },
  {
    id: "noto-serif-jp",
    label: "Noto Serif JP",
    description: "Google Fonts 配信の明朝系です。本文のメリハリと縦書きの見え方を比べる用途に向きます。",
    familyKind: "明朝系",
    sourceLabel: "Google Fonts",
    familyCss: '"Noto Serif JP", "Noto Serif CJK JP", var(--font-serif-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=Noto+Serif+JP:wght@300;400;500&display=swap",
    supportedWeights: [300, 400, 500]
  },
  {
    id: "noto-sans-jp",
    label: "Noto Sans JP",
    description: "Google Fonts 配信のゴシック系です。現行ゴシックより太さ感が合うか確認する用途に向きます。",
    familyKind: "ゴシック系",
    sourceLabel: "Google Fonts",
    familyCss: '"Noto Sans JP", "Noto Sans CJK JP", var(--font-sans-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=Noto+Sans+JP:wght@300;400;500&display=swap",
    supportedWeights: [300, 400, 500]
  },
  {
    id: "hiragino-mincho",
    label: "ヒラギノ明朝",
    description: "iPhone / iPad では端末ローカルのヒラギノ明朝系を優先します。",
    familyKind: "明朝系",
    sourceLabel: "Apple ローカル",
    familyCss: '"Hiragino Mincho ProN", "Hiragino Mincho Pro", "Yu Mincho", var(--font-serif-ja)',
    stylesheetHref: null,
    supportedWeights: null
  },
  {
    id: "hiragino-sans",
    label: "ヒラギノ角ゴ",
    description: "iPhone / iPad では端末ローカルのヒラギノ角ゴ系を優先します。",
    familyKind: "ゴシック系",
    sourceLabel: "Apple ローカル",
    familyCss: '"Hiragino Sans", "Hiragino Kaku Gothic ProN", "Hiragino Kaku Gothic Pro", var(--font-sans-ja)',
    stylesheetHref: null,
    supportedWeights: null
  },
  {
    id: "zen-kaku-gothic-new",
    label: "Zen Kaku Gothic New",
    description: "Google Fonts 配信のゴシック系です。硬質で均整の取れた見え方を比較できます。",
    familyKind: "ゴシック系",
    sourceLabel: "Google Fonts",
    familyCss: '"Zen Kaku Gothic New", var(--font-sans-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=Zen+Kaku+Gothic+New:wght@300;400;500&display=swap",
    supportedWeights: [300, 400, 500]
  },
  {
    id: "m-plus-1p",
    label: "M PLUS 1p",
    description: "Google Fonts 配信のゴシック系です。軽さと角の立ち方のバランスを比較できます。",
    familyKind: "ゴシック系",
    sourceLabel: "Google Fonts",
    familyCss: '"M PLUS 1p", var(--font-sans-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=M+PLUS+1p:wght@300;400;500&display=swap",
    supportedWeights: [300, 400, 500]
  },
  {
    id: "kosugi",
    label: "Kosugi",
    description: "Google Fonts 配信のゴシック系です。素直で細めの本文向けゴシックとして比較できます。",
    familyKind: "ゴシック系",
    sourceLabel: "Google Fonts",
    familyCss: '"Kosugi", var(--font-sans-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=Kosugi&display=swap",
    supportedWeights: [400]
  },
  {
    id: "hina-mincho",
    label: "Hina Mincho",
    description: "Google Fonts 配信の明朝系です。やわらかい筆致寄りの見え方を本文比較で確認できます。",
    familyKind: "明朝系",
    sourceLabel: "Google Fonts",
    familyCss: '"Hina Mincho", var(--font-serif-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=Hina+Mincho&display=swap",
    supportedWeights: [400]
  },
  {
    id: "zen-old-mincho",
    label: "Zen Old Mincho",
    description: "Google Fonts 配信の明朝系です。古典寄りの小説本文らしい見え方を比較できます。",
    familyKind: "明朝系",
    sourceLabel: "Google Fonts",
    familyCss: '"Zen Old Mincho", var(--font-serif-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=Zen+Old+Mincho:wght@400;500&display=swap",
    supportedWeights: [400, 500]
  },
  {
    id: "zen-antique",
    label: "Zen Antique",
    description: "Google Fonts 配信の明朝系です。骨太で装飾感のある字面を比較できます。",
    familyKind: "明朝系",
    sourceLabel: "Google Fonts",
    familyCss: '"Zen Antique", var(--font-serif-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=Zen+Antique&display=swap",
    supportedWeights: [400]
  },
  {
    id: "shippori-mincho",
    label: "Shippori Mincho",
    description: "Google Fonts 配信の明朝系です。やや現代的で細身の本文向け明朝として比較できます。",
    familyKind: "明朝系",
    sourceLabel: "Google Fonts",
    familyCss: '"Shippori Mincho", var(--font-serif-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=Shippori+Mincho:wght@400;500&display=swap",
    supportedWeights: [400, 500]
  },
  {
    id: "biz-udp-mincho",
    label: "BIZ UDPMincho",
    description: "Google Fonts 配信の明朝系です。UD 系の視認性重視な本文バランスを比較できます。",
    familyKind: "明朝系",
    sourceLabel: "Google Fonts",
    familyCss: '"BIZ UDPMincho", var(--font-serif-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=BIZ+UDPMincho:wght@400;700&display=swap",
    supportedWeights: [400]
  },
  {
    id: "biz-udp-gothic",
    label: "BIZ UDPGothic",
    description: "Google Fonts 配信のゴシック系です。UD 系の読みやすさを本文比較で確認できます。",
    familyKind: "ゴシック系",
    sourceLabel: "Google Fonts",
    familyCss: '"BIZ UDPGothic", var(--font-sans-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=BIZ+UDPGothic:wght@400;700&display=swap",
    supportedWeights: [400]
  },
  {
    id: "ibm-plex-sans-jp",
    label: "IBM Plex Sans JP",
    description: "Google Fonts 配信のゴシック系です。癖の少ないモダンな字面を本文比較に使えます。",
    familyKind: "ゴシック系",
    sourceLabel: "Google Fonts",
    familyCss: '"IBM Plex Sans JP", var(--font-sans-ja)',
    stylesheetHref: "https://fonts.googleapis.com/css2?family=IBM+Plex+Sans+JP:wght@300;400;500&display=swap",
    supportedWeights: [300, 400, 500]
  }
] as const;

const DEFAULT_READER_EXPERIMENTAL_FONT_OPTION = (() => {
  const defaultOption = READER_EXPERIMENTAL_FONT_OPTIONS.find((option) => option.id === DEFAULT_READER_EXPERIMENTAL_FONT_ID);
  if (!defaultOption) {
    throw new Error("DEFAULT_READER_EXPERIMENTAL_FONT_ID に対応する option が未定義です。");
  }

  return defaultOption;
})();

export type ReaderExperimentalFontId = (typeof READER_EXPERIMENTAL_FONT_OPTIONS)[number]["id"];
export type ReaderExperimentalFontOption = (typeof READER_EXPERIMENTAL_FONT_OPTIONS)[number];
export type ReaderExperimentalFontWeightOption = (typeof READER_EXPERIMENTAL_FONT_WEIGHT_OPTIONS)[number];

export function isReaderExperimentalFontId(value: unknown): value is ReaderExperimentalFontId {
  return typeof value === "string" && READER_EXPERIMENTAL_FONT_OPTIONS.some((option) => option.id === value);
}

export function isReaderExperimentalFontWeight(value: unknown): value is ReaderExperimentalFontWeight {
  return typeof value === "number" && READER_EXPERIMENTAL_FONT_WEIGHT_OPTIONS.some((option) => option.value === value);
}

export function getReaderDefaultFontFamilyCss(fontFamily: "mincho" | "gothic"): string {
  return fontFamily === "mincho" ? "var(--font-serif-ja)" : "var(--font-sans-ja)";
}

export function getReaderExperimentalFontOption(experimentalFontId: ReaderExperimentalFontId): ReaderExperimentalFontOption {
  return READER_EXPERIMENTAL_FONT_OPTIONS.find((option) => option.id === experimentalFontId) ?? DEFAULT_READER_EXPERIMENTAL_FONT_OPTION;
}

export function getReaderArticleFontFamilyCss(
  fontFamily: "mincho" | "gothic",
  experimentalFontId: ReaderExperimentalFontId
): string {
  const selectedOption = getReaderExperimentalFontOption(experimentalFontId);
  return selectedOption.familyCss ?? getReaderDefaultFontFamilyCss(fontFamily);
}

export function resolveReaderExperimentalFontWeight(
  experimentalFontId: ReaderExperimentalFontId,
  requestedWeight: ReaderExperimentalFontWeight
): ReaderExperimentalFontWeight {
  const selectedOption = getReaderExperimentalFontOption(experimentalFontId);
  const supportedWeights = selectedOption.supportedWeights as readonly ReaderExperimentalFontWeight[] | null;
  if (!supportedWeights || supportedWeights.includes(requestedWeight)) {
    return requestedWeight;
  }

  let closestWeight = supportedWeights[0];
  let smallestDistance = Math.abs(closestWeight - requestedWeight);
  for (const candidate of supportedWeights.slice(1)) {
    const distance = Math.abs(candidate - requestedWeight);
    if (distance < smallestDistance) {
      closestWeight = candidate;
      smallestDistance = distance;
      continue;
    }

    if (distance === smallestDistance && candidate < closestWeight) {
      closestWeight = candidate;
    }
  }

  return closestWeight;
}

export function getReaderArticleFontWeight(
  experimentalFontId: ReaderExperimentalFontId,
  requestedWeight: ReaderExperimentalFontWeight
): ReaderExperimentalFontWeight | null {
  if (experimentalFontId === DEFAULT_READER_EXPERIMENTAL_FONT_ID) {
    return null;
  }

  return resolveReaderExperimentalFontWeight(experimentalFontId, requestedWeight);
}
