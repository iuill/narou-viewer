import { describe, expect, it } from "vitest";
import {
  DEFAULT_READER_LOCAL_PREFERENCES,
  getReaderServerPreferencesKey,
  loadReaderLocalPreferences,
  saveReaderLocalPreferences
} from "../src/readerPreferences";

describe("readerPreferences", () => {
  it("loads local reader preferences with normalization and fallbacks", () => {
    const storage = {
      getItem() {
        return JSON.stringify({
          fontSizePx: 99,
          letterSpacingEm: -1,
          reverseTapPageNavigation: true,
          debugPageOverflow: true,
          speechEnabled: false,
          speechRate: 2.4,
          speechVoiceUri: "  voice:ja  ",
          speechPreferRubyText: false,
          speechDebugHighlight: true,
          experimentalFontId: "kosugi",
          experimentalFontWeight: 500
        });
      }
    };

    expect(loadReaderLocalPreferences(storage)).toEqual({
      fontSizePx: 36,
      letterSpacingEm: 0,
      reverseTapPageNavigation: true,
      debugPageOverflow: true,
      speechEnabled: false,
      speechRate: 2,
      speechVoiceUri: "voice:ja",
      speechPreferRubyText: false,
      speechDebugHighlight: true,
      experimentalFontId: "kosugi",
      experimentalFontWeight: 500
    });
  });

  it("falls back to defaults when local reader preferences are invalid", () => {
    const storage = {
      getItem() {
        return "{invalid";
      }
    };

    expect(loadReaderLocalPreferences(storage)).toEqual(DEFAULT_READER_LOCAL_PREFERENCES);
  });

  it("saves normalized local reader preferences", () => {
    let savedKey = "";
    let savedValue = "";
    const storage = {
      setItem(key: string, value: string) {
        savedKey = key;
        savedValue = value;
      }
    };

    saveReaderLocalPreferences(
        {
          fontSizePx: 13.4,
          letterSpacingEm: 0.236,
          reverseTapPageNavigation: true,
          debugPageOverflow: true,
          speechEnabled: false,
          speechRate: 0.44,
          speechVoiceUri: "voice:alt",
          speechPreferRubyText: false,
          speechDebugHighlight: true,
          experimentalFontId: "noto-serif-jp",
          experimentalFontWeight: 300
        },
        storage
      );

    expect(savedKey).toBe("narou-viewer.reader-local-preferences.v1");
    expect(JSON.parse(savedValue)).toEqual({
      fontSizePx: 14,
      letterSpacingEm: 0.24,
      reverseTapPageNavigation: true,
      debugPageOverflow: true,
      speechEnabled: false,
      speechRate: 0.5,
      speechVoiceUri: "voice:alt",
      speechPreferRubyText: false,
      speechDebugHighlight: true,
      experimentalFontId: "noto-serif-jp",
      experimentalFontWeight: 300
    });
  });

  it("does not throw when local reader preferences cannot be saved", () => {
    const storage = {
      setItem() {
        throw new Error("QuotaExceededError");
      }
    };

    expect(() =>
      saveReaderLocalPreferences(
        {
          fontSizePx: 20,
          letterSpacingEm: 0.08,
          reverseTapPageNavigation: false,
          debugPageOverflow: false,
          speechEnabled: true,
          speechRate: 1,
          speechVoiceUri: null,
          speechPreferRubyText: true,
          speechDebugHighlight: false,
          experimentalFontId: "none",
          experimentalFontWeight: 400
        },
        storage
      )
    ).not.toThrow();
  });

  it("creates stable keys for server reader preferences", () => {
    expect(
      getReaderServerPreferencesKey({
        readingMode: "horizontal",
        fontFamily: "gothic",
        theme: "forest"
      })
    ).toBe("horizontal:gothic:forest");
  });
});
