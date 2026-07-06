import { describe, expect, it } from "vitest";

import {
  DEFAULT_READER_EXPERIMENTAL_FONT_ID,
  READER_EXPERIMENTAL_FONT_OPTIONS,
  READER_EXPERIMENTAL_FONT_WEIGHT_OPTIONS,
  getReaderArticleFontFamilyCss,
  getReaderArticleFontWeight,
  getReaderExperimentalFontOption,
  isReaderExperimentalFontId,
  isReaderExperimentalFontWeight,
  resolveReaderExperimentalFontWeight
} from "../src/readerExperimentalFonts";

describe("readerExperimentalFonts", () => {
  it("ID 一覧が一意である", () => {
    const ids = READER_EXPERIMENTAL_FONT_OPTIONS.map((option) => option.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("既知の experimentalFontId を判定できる", () => {
    expect(isReaderExperimentalFontId("noto-serif-jp")).toBe(true);
    expect(isReaderExperimentalFontId("shippori-mincho")).toBe(true);
    expect(isReaderExperimentalFontId("kosugi")).toBe(true);
    expect(isReaderExperimentalFontId("hina-mincho")).toBe(true);
    expect(isReaderExperimentalFontId("zen-antique")).toBe(true);
    expect(isReaderExperimentalFontId("biz-udp-mincho")).toBe(true);
    expect(isReaderExperimentalFontId("biz-udp-gothic")).toBe(true);
    expect(isReaderExperimentalFontId("ibm-plex-sans-jp")).toBe(true);
    expect(isReaderExperimentalFontId("invalid-font")).toBe(false);
    expect(isReaderExperimentalFontId(null)).toBe(false);
  });

  it("既知の experimentalFontWeight を判定できる", () => {
    expect(READER_EXPERIMENTAL_FONT_WEIGHT_OPTIONS.map((option) => option.value)).toEqual([300, 400, 500]);
    expect(isReaderExperimentalFontWeight(300)).toBe(true);
    expect(isReaderExperimentalFontWeight(500)).toBe(true);
    expect(isReaderExperimentalFontWeight(700)).toBe(false);
    expect(isReaderExperimentalFontWeight("400")).toBe(false);
  });

  it("未知の ID では既定 option にフォールバックする", () => {
    expect(getReaderExperimentalFontOption("invalid-font" as never).id).toBe(DEFAULT_READER_EXPERIMENTAL_FONT_ID);
  });

  it("既定 option は配列先頭に依存せず id で解決する", () => {
    const defaultOption = READER_EXPERIMENTAL_FONT_OPTIONS.find((option) => option.id === DEFAULT_READER_EXPERIMENTAL_FONT_ID);
    expect(defaultOption).toBeTruthy();
    expect(getReaderExperimentalFontOption("invalid-font" as never)).toEqual(defaultOption);
  });

  it("既定設定と実験フォント設定から本文用 font-family を解決する", () => {
    expect(getReaderArticleFontFamilyCss("mincho", "none")).toBe("var(--font-serif-ja)");
    expect(getReaderArticleFontFamilyCss("gothic", "none")).toBe("var(--font-sans-ja)");
    expect(getReaderArticleFontFamilyCss("gothic", "noto-sans-jp")).toContain('"Noto Sans JP"');
    expect(getReaderArticleFontFamilyCss("mincho", "zen-old-mincho")).toContain('"Zen Old Mincho"');
    expect(getReaderArticleFontFamilyCss("mincho", "hina-mincho")).toContain('"Hina Mincho"');
    expect(getReaderArticleFontFamilyCss("gothic", "ibm-plex-sans-jp")).toContain('"IBM Plex Sans JP"');
  });

  it("フォントごとの対応太さから実際の font-weight を解決する", () => {
    expect(resolveReaderExperimentalFontWeight("noto-sans-jp", 300)).toBe(300);
    expect(resolveReaderExperimentalFontWeight("kosugi", 300)).toBe(400);
    expect(resolveReaderExperimentalFontWeight("biz-udp-mincho", 500)).toBe(400);
    expect(resolveReaderExperimentalFontWeight("biz-udp-gothic", 300)).toBe(400);
    expect(resolveReaderExperimentalFontWeight("shippori-mincho", 300)).toBe(400);
    expect(resolveReaderExperimentalFontWeight("shippori-mincho", 500)).toBe(500);
    expect(resolveReaderExperimentalFontWeight("ibm-plex-sans-jp", 500)).toBe(500);
    expect(resolveReaderExperimentalFontWeight("none", 500)).toBe(500);
  });

  it("対応太さを走査して最も近い値へ寄せる", () => {
    const targetOption = READER_EXPERIMENTAL_FONT_OPTIONS.find((option) => option.id === "noto-serif-jp");
    expect(targetOption).toBeTruthy();

    const originalWeights = targetOption?.supportedWeights;
    (targetOption as { supportedWeights: number[] }).supportedWeights = [500, 400];

    try {
      expect(resolveReaderExperimentalFontWeight("noto-serif-jp", 300)).toBe(400);
    } finally {
      (targetOption as { supportedWeights: typeof originalWeights }).supportedWeights = originalWeights;
    }
  });

  it("同距離なら軽い太さを優先する", () => {
    const targetOption = READER_EXPERIMENTAL_FONT_OPTIONS.find((option) => option.id === "noto-serif-jp");
    expect(targetOption).toBeTruthy();

    const originalWeights = targetOption?.supportedWeights;
    (targetOption as { supportedWeights: number[] }).supportedWeights = [500, 300];

    try {
      expect(resolveReaderExperimentalFontWeight("noto-serif-jp", 400)).toBe(300);
    } finally {
      (targetOption as { supportedWeights: typeof originalWeights }).supportedWeights = originalWeights;
    }
  });

  it("標準設定では本文用 font-weight を適用しない", () => {
    expect(getReaderArticleFontWeight("none", 500)).toBeNull();
    expect(getReaderArticleFontWeight("noto-sans-jp", 500)).toBe(500);
    expect(getReaderArticleFontWeight("kosugi", 300)).toBe(400);
  });
});
