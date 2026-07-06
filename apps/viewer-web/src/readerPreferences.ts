import {
  DEFAULT_READER_EXPERIMENTAL_FONT_ID,
  isReaderExperimentalFontId,
  isReaderExperimentalFontWeight,
  type ReaderExperimentalFontId,
  type ReaderExperimentalFontWeight
} from "./readerExperimentalFonts";
import type { ReaderFontFamily, ReaderPreferencesResponse, ReaderTheme, ReadingMode } from "./features/reader/types";
export type { ReaderExperimentalFontId, ReaderExperimentalFontWeight } from "./readerExperimentalFonts";
export type { ReaderFontFamily, ReaderTheme, ReadingMode } from "./features/reader/types";

export type ReaderServerPreferences = {
  readingMode: ReadingMode;
  fontFamily: ReaderFontFamily;
  theme: ReaderTheme;
};

export type ReaderLocalPreferences = {
  fontSizePx: number;
  letterSpacingEm: number;
  reverseTapPageNavigation: boolean;
  debugPageOverflow: boolean;
  speechEnabled: boolean;
  speechRate: number;
  speechVoiceUri: string | null;
  speechPreferRubyText: boolean;
  speechDebugHighlight: boolean;
  experimentalFontId: ReaderExperimentalFontId;
  experimentalFontWeight: ReaderExperimentalFontWeight;
};

export const DEFAULT_READER_SERVER_PREFERENCES: ReaderServerPreferences = {
  readingMode: "vertical",
  fontFamily: "mincho",
  theme: "classic"
};

export const DEFAULT_READER_LOCAL_PREFERENCES: ReaderLocalPreferences = {
  fontSizePx: 20,
  letterSpacingEm: 0.08,
  reverseTapPageNavigation: false,
  debugPageOverflow: false,
  speechEnabled: true,
  speechRate: 1,
  speechVoiceUri: null,
  speechPreferRubyText: true,
  speechDebugHighlight: false,
  experimentalFontId: DEFAULT_READER_EXPERIMENTAL_FONT_ID,
  experimentalFontWeight: 400
};

const READER_LOCAL_PREFERENCES_STORAGE_KEY = "narou-viewer.reader-local-preferences.v1";
const READER_FONT_SIZE_MIN_PX = 14;
const READER_FONT_SIZE_MAX_PX = 36;
const READER_LETTER_SPACING_MIN_EM = 0;
const READER_LETTER_SPACING_MAX_EM = 0.24;
const READER_SPEECH_RATE_MIN = 0.5;
const READER_SPEECH_RATE_MAX = 2;

function getDefaultStorage(): Storage | null {
  return typeof window === "undefined" ? null : window.localStorage;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function normalizeReaderFontSizePx(value: unknown): number | null {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }

  return Math.min(READER_FONT_SIZE_MAX_PX, Math.max(READER_FONT_SIZE_MIN_PX, Math.round(value)));
}

function normalizeReaderLetterSpacingEm(value: unknown): number | null {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }

  const normalized = Math.min(READER_LETTER_SPACING_MAX_EM, Math.max(READER_LETTER_SPACING_MIN_EM, value));
  return Math.round(normalized * 100) / 100;
}

function normalizeReverseTapPageNavigation(value: unknown): boolean | null {
  return typeof value === "boolean" ? value : null;
}

function normalizeDebugPageOverflow(value: unknown): boolean | null {
  return typeof value === "boolean" ? value : null;
}

function normalizeSpeechEnabled(value: unknown): boolean | null {
  return typeof value === "boolean" ? value : null;
}

function normalizeSpeechRate(value: unknown): number | null {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }

  const normalized = Math.min(READER_SPEECH_RATE_MAX, Math.max(READER_SPEECH_RATE_MIN, value));
  return Math.round(normalized * 100) / 100;
}

function normalizeSpeechVoiceUri(value: unknown): string | null {
  if (typeof value !== "string") {
    return null;
  }

  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

function normalizeSpeechPreferRubyText(value: unknown): boolean | null {
  return typeof value === "boolean" ? value : null;
}

function normalizeSpeechDebugHighlight(value: unknown): boolean | null {
  return typeof value === "boolean" ? value : null;
}

function normalizeReaderExperimentalFontId(value: unknown): ReaderExperimentalFontId | null {
  return isReaderExperimentalFontId(value) ? value : null;
}

function normalizeReaderExperimentalFontWeight(value: unknown): ReaderExperimentalFontWeight | null {
  return isReaderExperimentalFontWeight(value) ? value : null;
}

export function loadReaderLocalPreferences(storage: Pick<Storage, "getItem"> | null = getDefaultStorage()): ReaderLocalPreferences {
  if (!storage) {
    return DEFAULT_READER_LOCAL_PREFERENCES;
  }

  try {
    const raw = storage.getItem(READER_LOCAL_PREFERENCES_STORAGE_KEY);
    if (!raw) {
      return DEFAULT_READER_LOCAL_PREFERENCES;
    }

    const parsed = JSON.parse(raw) as unknown;
    if (!isRecord(parsed)) {
      return DEFAULT_READER_LOCAL_PREFERENCES;
    }

    return {
      fontSizePx: normalizeReaderFontSizePx(parsed.fontSizePx) ?? DEFAULT_READER_LOCAL_PREFERENCES.fontSizePx,
      letterSpacingEm:
        normalizeReaderLetterSpacingEm(parsed.letterSpacingEm) ?? DEFAULT_READER_LOCAL_PREFERENCES.letterSpacingEm,
      reverseTapPageNavigation:
        normalizeReverseTapPageNavigation(parsed.reverseTapPageNavigation) ??
        DEFAULT_READER_LOCAL_PREFERENCES.reverseTapPageNavigation,
      debugPageOverflow:
        normalizeDebugPageOverflow(parsed.debugPageOverflow) ?? DEFAULT_READER_LOCAL_PREFERENCES.debugPageOverflow,
      speechEnabled: normalizeSpeechEnabled(parsed.speechEnabled) ?? DEFAULT_READER_LOCAL_PREFERENCES.speechEnabled,
      speechRate: normalizeSpeechRate(parsed.speechRate) ?? DEFAULT_READER_LOCAL_PREFERENCES.speechRate,
      speechVoiceUri:
        normalizeSpeechVoiceUri(parsed.speechVoiceUri) ?? DEFAULT_READER_LOCAL_PREFERENCES.speechVoiceUri,
      speechPreferRubyText:
        normalizeSpeechPreferRubyText(parsed.speechPreferRubyText) ??
        DEFAULT_READER_LOCAL_PREFERENCES.speechPreferRubyText,
      speechDebugHighlight:
        normalizeSpeechDebugHighlight(parsed.speechDebugHighlight) ??
        DEFAULT_READER_LOCAL_PREFERENCES.speechDebugHighlight,
      experimentalFontId:
        normalizeReaderExperimentalFontId(parsed.experimentalFontId) ??
        DEFAULT_READER_LOCAL_PREFERENCES.experimentalFontId,
      experimentalFontWeight:
        normalizeReaderExperimentalFontWeight(parsed.experimentalFontWeight) ??
        DEFAULT_READER_LOCAL_PREFERENCES.experimentalFontWeight
    };
  } catch {
    return DEFAULT_READER_LOCAL_PREFERENCES;
  }
}

export function saveReaderLocalPreferences(
  preferences: ReaderLocalPreferences,
  storage: Pick<Storage, "setItem"> | null = getDefaultStorage()
): void {
  if (!storage) {
    return;
  }

  const normalizedPreferences: ReaderLocalPreferences = {
    fontSizePx: normalizeReaderFontSizePx(preferences.fontSizePx) ?? DEFAULT_READER_LOCAL_PREFERENCES.fontSizePx,
    letterSpacingEm:
      normalizeReaderLetterSpacingEm(preferences.letterSpacingEm) ?? DEFAULT_READER_LOCAL_PREFERENCES.letterSpacingEm,
    reverseTapPageNavigation:
      normalizeReverseTapPageNavigation(preferences.reverseTapPageNavigation) ??
      DEFAULT_READER_LOCAL_PREFERENCES.reverseTapPageNavigation,
    debugPageOverflow:
      normalizeDebugPageOverflow(preferences.debugPageOverflow) ?? DEFAULT_READER_LOCAL_PREFERENCES.debugPageOverflow,
    speechEnabled: normalizeSpeechEnabled(preferences.speechEnabled) ?? DEFAULT_READER_LOCAL_PREFERENCES.speechEnabled,
    speechRate: normalizeSpeechRate(preferences.speechRate) ?? DEFAULT_READER_LOCAL_PREFERENCES.speechRate,
    speechVoiceUri: normalizeSpeechVoiceUri(preferences.speechVoiceUri) ?? DEFAULT_READER_LOCAL_PREFERENCES.speechVoiceUri,
    speechPreferRubyText:
      normalizeSpeechPreferRubyText(preferences.speechPreferRubyText) ??
      DEFAULT_READER_LOCAL_PREFERENCES.speechPreferRubyText,
    speechDebugHighlight:
      normalizeSpeechDebugHighlight(preferences.speechDebugHighlight) ??
      DEFAULT_READER_LOCAL_PREFERENCES.speechDebugHighlight,
    experimentalFontId:
      normalizeReaderExperimentalFontId(preferences.experimentalFontId) ??
      DEFAULT_READER_LOCAL_PREFERENCES.experimentalFontId,
    experimentalFontWeight:
      normalizeReaderExperimentalFontWeight(preferences.experimentalFontWeight) ??
      DEFAULT_READER_LOCAL_PREFERENCES.experimentalFontWeight
  };

  try {
    storage.setItem(READER_LOCAL_PREFERENCES_STORAGE_KEY, JSON.stringify(normalizedPreferences));
  } catch {
    return;
  }
}

export function getReaderServerPreferencesKey(preferences: ReaderServerPreferences): string {
  return `${preferences.readingMode}:${preferences.fontFamily}:${preferences.theme}`;
}

export function toReaderServerPreferences(response: ReaderPreferencesResponse): ReaderServerPreferences {
  return {
    readingMode: response.readingMode,
    fontFamily: response.fontFamily,
    theme: response.theme
  };
}
