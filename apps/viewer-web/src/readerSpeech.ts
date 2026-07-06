import type { ReaderDocument, ReaderInlineToken } from "./readerDocument";
import { countGraphemes } from "./readerPosition";

export type ReaderSpeechChunk = {
  chunkIndex: number;
  startPosition: number;
  endPosition: number;
  text: string;
  estimatedDurationMs: number | null;
  voiceHint: string | null;
  speakerHint: string | null;
  positionAnchors?: ReaderSpeechPositionAnchor[];
};

export type ReaderSpeechPositionAnchor = {
  charIndex: number;
  position: number;
};

export type ReaderSpeechRequestOptions = {
  rate: number;
  voiceURI: string | null;
  preferRubyText: boolean;
};

export type ReaderSpeechEngineState = "idle" | "preparing" | "playing" | "paused" | "stopped";
export type ReaderSpeechProgressSource = "start" | "boundary" | "end";

export type ReaderSpeechEngineEvent =
  | { type: "chunkReady"; requestId: string | null; chunkIndex: number; durationMs: number | null }
  | {
      type: "progress";
      requestId: string | null;
      chunkIndex: number;
      position: number;
      pageChanged: boolean;
      source: ReaderSpeechProgressSource;
      charIndex: number | null;
      elapsedTimeMs: number | null;
    }
  | { type: "chunkEnd"; requestId: string | null; chunkIndex: number; endPosition: number }
  | { type: "error"; requestId: string | null; chunkIndex: number | null; message: string }
  | { type: "state"; requestId: string | null; state: ReaderSpeechEngineState };

export type ReaderSpeechEngineListener = (event: ReaderSpeechEngineEvent) => void;

export type ReaderSpeechEngine = {
  kind: "browser" | "server";
  prepare(chunks: ReaderSpeechChunk[], options: ReaderSpeechRequestOptions): Promise<void>;
  play(chunkIndex: number, options?: ReaderSpeechPlayOptions): Promise<void>;
  pause(): Promise<void>;
  resume(): Promise<void>;
  stop(): Promise<void>;
  dispose(): Promise<void>;
  subscribe(listener: ReaderSpeechEngineListener): () => void;
};

export type ReaderSpeechPlayOptions = {
  startPosition?: number | null;
};

export type ReaderSpeechVoiceOption = {
  voiceURI: string;
  name: string;
  lang: string;
  default: boolean;
  localService: boolean;
};

type ReaderSpeechFragment = {
  speechText: string;
  startPosition: number;
  endPosition: number;
};

type BuildReaderSpeechChunksOptions = {
  preferRubyText?: boolean;
  maxChunkGraphemes?: number;
  targetChunkGraphemes?: number;
};

const DEFAULT_READER_SPEECH_RATE = 1;
const DEFAULT_READER_SPEECH_MAX_CHUNK_GRAPHEMES = 220;
const DEFAULT_READER_SPEECH_TARGET_CHUNK_GRAPHEMES = 160;
const READER_SPEECH_ESTIMATED_DURATION_PER_GRAPHEME_MS = 90;

const graphemeSegmenter =
  typeof Intl !== "undefined" && "Segmenter" in Intl ? new Intl.Segmenter("ja", { granularity: "grapheme" }) : null;

function splitIntoGraphemes(text: string): string[] {
  if (text.length === 0) {
    return [];
  }

  if (graphemeSegmenter) {
    return Array.from(graphemeSegmenter.segment(text), (segment) => segment.segment);
  }

  return Array.from(text);
}

function getGraphemeOffsetFromCodeUnitOffset(text: string, codeUnitOffset: number): number {
  if (text.length === 0 || codeUnitOffset <= 0) {
    return 0;
  }

  if (graphemeSegmenter) {
    let graphemeOffset = 0;
    for (const segment of graphemeSegmenter.segment(text)) {
      if (segment.index >= codeUnitOffset) {
        return graphemeOffset;
      }

      graphemeOffset += 1;
    }

    return graphemeOffset;
  }

  return Array.from(text.slice(0, codeUnitOffset)).length;
}

function getCodeUnitOffsetFromGraphemeOffset(text: string, graphemeOffset: number): number {
  const normalizedGraphemeOffset = Number.isFinite(graphemeOffset) ? Math.max(0, Math.ceil(graphemeOffset)) : 0;
  if (text.length === 0 || normalizedGraphemeOffset <= 0) {
    return 0;
  }

  if (graphemeSegmenter) {
    let currentGraphemeOffset = 0;
    for (const segment of graphemeSegmenter.segment(text)) {
      if (currentGraphemeOffset >= normalizedGraphemeOffset) {
        return segment.index;
      }

      currentGraphemeOffset += 1;
    }

    return text.length;
  }

  return Array.from(text).slice(0, normalizedGraphemeOffset).join("").length;
}

function normalizeSpeechSynthesisElapsedTimeMs(elapsedTime: number): number | null {
  if (!Number.isFinite(elapsedTime)) {
    return null;
  }

  const elapsedTimeMs = elapsedTime > 1000 ? elapsedTime : elapsedTime * 1000;
  return Math.round(elapsedTimeMs);
}

function normalizeReaderSpeechText(text: string): string {
  return text.replace(/\n{3,}/g, "\n\n").trim();
}

function getNormalizedSpeechCharIndex(rawText: string, rawIndex: number): number {
  const normalizedFullText = rawText.replace(/\n{3,}/g, "\n\n");
  const leadingTrimLength = normalizedFullText.length - normalizedFullText.trimStart().length;
  const finalTextLength = normalizedFullText.trim().length;
  const normalizedPrefixLength = rawText.slice(0, Math.max(0, rawIndex)).replace(/\n{3,}/g, "\n\n").length;

  return Math.min(Math.max(normalizedPrefixLength - leadingTrimLength, 0), finalTextLength);
}

function clampReaderSpeechRate(rate: number): number {
  if (!Number.isFinite(rate)) {
    return DEFAULT_READER_SPEECH_RATE;
  }

  return Math.min(Math.max(Math.round(rate * 100) / 100, 0.5), 2);
}

function createReaderSpeechFragment(text: string, startPosition: number, endPosition: number): ReaderSpeechFragment | null {
  if (endPosition <= startPosition || text.length === 0) {
    return null;
  }

  return {
    speechText: text,
    startPosition,
    endPosition
  };
}

function appendFragmentWithBoundary(
  fragments: ReaderSpeechFragment[],
  fragment: ReaderSpeechFragment | null,
  boundary: "" | "\n"
): void {
  if (!fragment) {
    return;
  }

  fragments.push({
    ...fragment,
    speechText: `${fragment.speechText}${boundary}`
  });
}

function tokenizeInlineSpeech(
  tokens: ReaderInlineToken[],
  fragments: ReaderSpeechFragment[],
  cursor: { value: number },
  preferRubyText: boolean
): void {
  for (const token of tokens) {
    if (token.type === "text" || token.type === "tcy") {
      const length = countGraphemes(token.text);
      const start = cursor.value;
      cursor.value += length;
      const fragment = createReaderSpeechFragment(token.text, start, cursor.value);
      if (fragment) {
        fragments.push(fragment);
      }
      continue;
    }

    if (token.type === "ruby") {
      const sourceLength = countGraphemes(token.text);
      const start = cursor.value;
      cursor.value += sourceLength;
      const fragment = createReaderSpeechFragment(preferRubyText ? token.ruby : token.text, start, cursor.value);
      if (fragment) {
        fragments.push(fragment);
      }
      continue;
    }

    if (token.type === "lineBreak") {
      const start = cursor.value;
      cursor.value += 1;
      const fragment = createReaderSpeechFragment("\n", start, cursor.value);
      if (fragment) {
        fragments.push(fragment);
      }
      continue;
    }

    tokenizeInlineSpeech(token.children, fragments, cursor, preferRubyText);
  }
}

function buildReaderSpeechFragments(
  readerDocument: ReaderDocument,
  options: Pick<BuildReaderSpeechChunksOptions, "preferRubyText"> = {}
): ReaderSpeechFragment[] {
  const preferRubyText = options.preferRubyText !== false;
  const fragments: ReaderSpeechFragment[] = [];
  const cursor = { value: 0 };

  for (const block of readerDocument.blocks) {
    if (block.type === "meta" || block.type === "title") {
      const start = cursor.value;
      cursor.value += countGraphemes(block.text);
      appendFragmentWithBoundary(fragments, createReaderSpeechFragment(block.text, start, cursor.value), "\n");
      continue;
    }

    if (block.type === "paragraph") {
      const paragraphStartIndex = fragments.length;
      tokenizeInlineSpeech(block.inlines, fragments, cursor, preferRubyText);
      const lastFragment = fragments[fragments.length - 1];
      if (lastFragment && fragments.length > paragraphStartIndex) {
        lastFragment.speechText = `${lastFragment.speechText}\n`;
      }
      continue;
    }

    if (block.type === "image") {
      cursor.value += 1;
      continue;
    }

    const start = cursor.value;
    cursor.value += 1;
    appendFragmentWithBoundary(fragments, createReaderSpeechFragment(block.plainText, start, cursor.value), "\n");
  }

  return fragments.filter((fragment) => fragment.speechText.length > 0);
}

function splitOversizedFragment(fragment: ReaderSpeechFragment, maxChunkGraphemes: number): ReaderSpeechFragment[] {
  const speechGraphemes = splitIntoGraphemes(fragment.speechText);
  const speechLength = speechGraphemes.length;
  const positionLength = fragment.endPosition - fragment.startPosition;
  if (speechLength <= maxChunkGraphemes && positionLength <= maxChunkGraphemes) {
    return [fragment];
  }

  const totalParts = Math.max(
    Math.ceil(Math.max(speechLength, 1) / maxChunkGraphemes),
    Math.ceil(Math.max(positionLength, 1) / maxChunkGraphemes)
  );
  const parts: ReaderSpeechFragment[] = [];

  for (let index = 0; index < totalParts; index += 1) {
    const startPositionOffset = Math.floor((positionLength * index) / totalParts);
    const endPositionOffset = Math.floor((positionLength * (index + 1)) / totalParts);
    const startSpeechOffset = Math.floor((speechLength * index) / totalParts);
    const endSpeechOffset = Math.floor((speechLength * (index + 1)) / totalParts);
    const speechText = speechGraphemes.slice(startSpeechOffset, endSpeechOffset).join("");
    const nextFragment = createReaderSpeechFragment(
      speechText,
      fragment.startPosition + startPositionOffset,
      fragment.startPosition + endPositionOffset
    );
    if (nextFragment) {
      parts.push(nextFragment);
    }
  }

  return parts;
}

function estimateChunkDurationMs(text: string, rate: number = DEFAULT_READER_SPEECH_RATE): number | null {
  const graphemeCount = countGraphemes(text);
  if (graphemeCount <= 0) {
    return null;
  }

  return Math.max(500, Math.round((graphemeCount * READER_SPEECH_ESTIMATED_DURATION_PER_GRAPHEME_MS) / clampReaderSpeechRate(rate)));
}

function buildReaderSpeechChunkTextAndAnchors(fragments: ReaderSpeechFragment[]): {
  text: string;
  positionAnchors: ReaderSpeechPositionAnchor[];
} {
  const rawText = fragments.map((fragment) => fragment.speechText).join("");
  const text = normalizeReaderSpeechText(rawText);
  const positionAnchors: ReaderSpeechPositionAnchor[] = [];
  let rawOffset = 0;

  for (const fragment of fragments) {
    const rawStart = rawOffset;
    const rawEnd = rawStart + fragment.speechText.length;
    positionAnchors.push({
      charIndex: getNormalizedSpeechCharIndex(rawText, rawStart),
      position: fragment.startPosition
    });
    positionAnchors.push({
      charIndex: getNormalizedSpeechCharIndex(rawText, rawEnd),
      position: fragment.endPosition
    });
    rawOffset = rawEnd;
  }

  const normalizedAnchors = positionAnchors
    .map((anchor) => ({
      charIndex: Math.min(Math.max(anchor.charIndex, 0), text.length),
      position: anchor.position
    }))
    .sort((left, right) => left.charIndex - right.charIndex || left.position - right.position);

  const dedupedAnchors: ReaderSpeechPositionAnchor[] = [];
  for (const anchor of normalizedAnchors) {
    const previous = dedupedAnchors[dedupedAnchors.length - 1];
    if (previous && previous.charIndex === anchor.charIndex) {
      previous.position = Math.max(previous.position, anchor.position);
      continue;
    }

    dedupedAnchors.push(anchor);
  }

  return { text, positionAnchors: dedupedAnchors };
}

export function buildReaderSpeechChunks(
  readerDocument: ReaderDocument,
  options: BuildReaderSpeechChunksOptions = {}
): ReaderSpeechChunk[] {
  const maxChunkGraphemes = Math.max(40, Math.round(options.maxChunkGraphemes ?? DEFAULT_READER_SPEECH_MAX_CHUNK_GRAPHEMES));
  const targetChunkGraphemes = Math.min(
    maxChunkGraphemes,
    Math.max(20, Math.round(options.targetChunkGraphemes ?? DEFAULT_READER_SPEECH_TARGET_CHUNK_GRAPHEMES))
  );
  const fragments = buildReaderSpeechFragments(readerDocument, options).flatMap((fragment) =>
    splitOversizedFragment(fragment, maxChunkGraphemes)
  );
  const chunks: ReaderSpeechChunk[] = [];
  let currentFragments: ReaderSpeechFragment[] = [];
  let currentLength = 0;

  const flush = () => {
    if (currentFragments.length === 0) {
      return;
    }

    const { text, positionAnchors } = buildReaderSpeechChunkTextAndAnchors(currentFragments);
    if (text.length === 0) {
      currentFragments = [];
      currentLength = 0;
      return;
    }

    const startPosition = currentFragments[0]?.startPosition;
    const endPosition = currentFragments[currentFragments.length - 1]?.endPosition;
    chunks.push({
      chunkIndex: chunks.length,
      startPosition,
      endPosition,
      text,
      estimatedDurationMs: estimateChunkDurationMs(text),
      voiceHint: null,
      speakerHint: null,
      positionAnchors
    });
    currentFragments = [];
    currentLength = 0;
  };

  for (const fragment of fragments) {
    const fragmentLength = countGraphemes(fragment.speechText);
    if (currentFragments.length > 0 && currentLength + fragmentLength > maxChunkGraphemes) {
      flush();
    }

    currentFragments.push(fragment);
    currentLength += fragmentLength;

    if (currentLength >= targetChunkGraphemes && fragment.speechText.endsWith("\n")) {
      flush();
    }
  }

  flush();

  return chunks;
}

export function findReaderSpeechChunkIndex(chunks: ReaderSpeechChunk[], position: number): number | null {
  if (chunks.length === 0) {
    return null;
  }

  const normalizedPosition = Math.max(0, position);

  for (const chunk of chunks) {
    if (normalizedPosition < chunk.endPosition) {
      return chunk.chunkIndex;
    }
  }

  return chunks[chunks.length - 1]?.chunkIndex ?? null;
}

export function getReaderSpeechPositionForCharIndex(chunk: ReaderSpeechChunk, charIndex: number): number {
  const normalizedCharIndex = Number.isFinite(charIndex) ? Math.round(charIndex) : 0;
  const clampedCharIndex = Math.min(Math.max(normalizedCharIndex, 0), chunk.text.length);
  const anchors = chunk.positionAnchors ?? [];
  if (anchors.length >= 2) {
    const sortedAnchors = [...anchors].sort((left, right) => left.charIndex - right.charIndex || left.position - right.position);
    const firstAnchor = sortedAnchors[0];
    if (!firstAnchor) {
      return chunk.startPosition;
    }
    if (clampedCharIndex <= firstAnchor.charIndex) {
      return firstAnchor.position;
    }

    for (let index = 1; index < sortedAnchors.length; index += 1) {
      const previousAnchor = sortedAnchors[index - 1];
      const nextAnchor = sortedAnchors[index];
      if (!previousAnchor || !nextAnchor) {
        continue;
      }
      if (clampedCharIndex > nextAnchor.charIndex) {
        continue;
      }

      if (nextAnchor.charIndex <= previousAnchor.charIndex || nextAnchor.position <= previousAnchor.position) {
        return nextAnchor.position;
      }

      const intervalText = chunk.text.slice(previousAnchor.charIndex, nextAnchor.charIndex);
      const intervalGraphemes = countGraphemes(intervalText);
      if (intervalGraphemes <= 0) {
        return nextAnchor.position;
      }

      const charIndexInInterval = clampedCharIndex - previousAnchor.charIndex;
      const graphemeOffset = getGraphemeOffsetFromCodeUnitOffset(intervalText, charIndexInInterval);
      const positionOffset = Math.floor(((nextAnchor.position - previousAnchor.position) * graphemeOffset) / intervalGraphemes);
      return Math.min(nextAnchor.position, Math.max(previousAnchor.position, previousAnchor.position + positionOffset));
    }

    return sortedAnchors.at(-1)?.position ?? chunk.endPosition;
  }

  const positionLength = chunk.endPosition - chunk.startPosition;
  if (positionLength <= 0 || chunk.text.length === 0) {
    return chunk.startPosition;
  }

  const graphemeLength = countGraphemes(chunk.text);
  if (graphemeLength <= 0) {
    return chunk.startPosition;
  }

  const graphemeOffset = getGraphemeOffsetFromCodeUnitOffset(chunk.text, clampedCharIndex);
  const positionOffset = Math.floor((positionLength * graphemeOffset) / graphemeLength);

  return Math.min(chunk.endPosition, Math.max(chunk.startPosition, chunk.startPosition + positionOffset));
}

export function getReaderSpeechCharIndexForPosition(chunk: ReaderSpeechChunk, position: number): number {
  const normalizedPosition = Number.isFinite(position) ? Math.round(position) : chunk.startPosition;
  const clampedPosition = Math.min(Math.max(normalizedPosition, chunk.startPosition), chunk.endPosition);
  const anchors = chunk.positionAnchors ?? [];
  if (anchors.length >= 2) {
    const sortedAnchors = [...anchors].sort((left, right) => left.charIndex - right.charIndex || left.position - right.position);
    const firstAnchor = sortedAnchors[0];
    if (!firstAnchor) {
      return 0;
    }
    if (clampedPosition <= firstAnchor.position) {
      return Math.min(Math.max(firstAnchor.charIndex, 0), chunk.text.length);
    }

    for (let index = 1; index < sortedAnchors.length; index += 1) {
      const previousAnchor = sortedAnchors[index - 1];
      const nextAnchor = sortedAnchors[index];
      if (!previousAnchor || !nextAnchor) {
        continue;
      }
      if (clampedPosition > nextAnchor.position) {
        continue;
      }

      if (nextAnchor.position <= previousAnchor.position || nextAnchor.charIndex <= previousAnchor.charIndex) {
        return Math.min(Math.max(nextAnchor.charIndex, 0), chunk.text.length);
      }

      const intervalText = chunk.text.slice(previousAnchor.charIndex, nextAnchor.charIndex);
      const intervalGraphemes = countGraphemes(intervalText);
      if (intervalGraphemes <= 0) {
        return Math.min(Math.max(nextAnchor.charIndex, 0), chunk.text.length);
      }

      const positionOffset = clampedPosition - previousAnchor.position;
      const positionLength = nextAnchor.position - previousAnchor.position;
      const graphemeOffset = Math.ceil((intervalGraphemes * positionOffset) / positionLength);
      const charIndexInInterval = getCodeUnitOffsetFromGraphemeOffset(intervalText, graphemeOffset);
      return Math.min(
        Math.max(previousAnchor.charIndex + charIndexInInterval, previousAnchor.charIndex),
        nextAnchor.charIndex,
        chunk.text.length
      );
    }

    return Math.min(Math.max(sortedAnchors.at(-1)?.charIndex ?? chunk.text.length, 0), chunk.text.length);
  }

  const positionLength = chunk.endPosition - chunk.startPosition;
  if (positionLength <= 0 || chunk.text.length === 0) {
    return 0;
  }

  const graphemeLength = countGraphemes(chunk.text);
  if (graphemeLength <= 0) {
    return 0;
  }

  const positionOffset = clampedPosition - chunk.startPosition;
  const graphemeOffset = Math.ceil((graphemeLength * positionOffset) / positionLength);
  return Math.min(getCodeUnitOffsetFromGraphemeOffset(chunk.text, graphemeOffset), chunk.text.length);
}

function createReaderSpeechPlaybackChunk(
  chunk: ReaderSpeechChunk,
  startPosition: number | null | undefined
): { chunk: ReaderSpeechChunk; charIndexOffset: number } {
  if (
    typeof startPosition !== "number" ||
    !Number.isFinite(startPosition) ||
    startPosition <= chunk.startPosition ||
    startPosition >= chunk.endPosition ||
    chunk.text.length === 0
  ) {
    return { chunk, charIndexOffset: 0 };
  }

  const startCharIndex = getReaderSpeechCharIndexForPosition(chunk, startPosition);
  if (startCharIndex <= 0 || startCharIndex >= chunk.text.length) {
    return { chunk, charIndexOffset: 0 };
  }

  const playbackText = chunk.text.slice(startCharIndex);
  const playbackStartPosition = getReaderSpeechPositionForCharIndex(chunk, startCharIndex);
  const sortedAnchors = [...(chunk.positionAnchors ?? [])].sort(
    (left, right) => left.charIndex - right.charIndex || left.position - right.position
  );
  const positionAnchors =
    sortedAnchors.length > 0
      ? [
          { charIndex: 0, position: playbackStartPosition },
          ...sortedAnchors
            .filter((anchor) => anchor.charIndex > startCharIndex)
            .map((anchor) => ({
              charIndex: anchor.charIndex - startCharIndex,
              position: anchor.position
            }))
        ]
      : undefined;
  const lastAnchor = positionAnchors?.[positionAnchors.length - 1] ?? null;
  const adjustedPositionAnchors =
    positionAnchors && (!lastAnchor || lastAnchor.charIndex < playbackText.length || lastAnchor.position < chunk.endPosition)
      ? [...positionAnchors, { charIndex: playbackText.length, position: chunk.endPosition }]
      : positionAnchors;

  return {
    chunk: {
      ...chunk,
      startPosition: playbackStartPosition,
      text: playbackText,
      positionAnchors: adjustedPositionAnchors
    },
    charIndexOffset: startCharIndex
  };
}

type ReaderSpeechWindow = Pick<Window, "speechSynthesis"> & {
  SpeechSynthesisUtterance?: typeof SpeechSynthesisUtterance;
};

function getSpeechSynthesisUtteranceConstructor(
  target: ReaderSpeechWindow | null = typeof window === "undefined" ? null : (window as ReaderSpeechWindow)
): typeof SpeechSynthesisUtterance | null {
  if (!target) {
    return null;
  }

  return typeof target.SpeechSynthesisUtterance === "function" ? target.SpeechSynthesisUtterance : null;
}

export function isBrowserReaderSpeechSupported(
  target: ReaderSpeechWindow | null = typeof window === "undefined" ? null : (window as ReaderSpeechWindow)
): boolean {
  if (!target) {
    return false;
  }

  return typeof target.speechSynthesis !== "undefined" && getSpeechSynthesisUtteranceConstructor(target) !== null;
}

function getDefaultReaderSpeechVoice(voices: SpeechSynthesisVoice[]): SpeechSynthesisVoice | null {
  return (
    voices.find((voice) => isJapaneseReaderSpeechVoice(voice)) ??
    voices.find((voice) => voice.default) ??
    voices[0] ??
    null
  );
}

function normalizeReaderSpeechVoiceLang(lang: string): string {
  return lang.trim().replaceAll("_", "-").toLowerCase();
}

function isJapaneseReaderSpeechVoice(voice: Pick<SpeechSynthesisVoice, "lang">): boolean {
  const normalizedLang = normalizeReaderSpeechVoiceLang(voice.lang);
  return normalizedLang === "ja-jp" || normalizedLang.startsWith("ja-jp-");
}

export function getBrowserReaderSpeechVoices(
  speechSynthesis: Pick<SpeechSynthesis, "getVoices"> | null =
    typeof window === "undefined" ? null : window.speechSynthesis
): ReaderSpeechVoiceOption[] {
  if (!speechSynthesis) {
    return [];
  }

  const uniqueVoices = new Map<string, ReaderSpeechVoiceOption>();

  for (const voice of speechSynthesis.getVoices()) {
    if (!isJapaneseReaderSpeechVoice(voice)) {
      continue;
    }

    const normalizedVoice = {
      voiceURI: voice.voiceURI,
      name: voice.name,
      lang: voice.lang,
      default: voice.default,
      localService: voice.localService
    };
    const key = normalizedVoice.voiceURI || [normalizedVoice.name, normalizedVoice.lang].join("|");
    if (!uniqueVoices.has(key)) {
      uniqueVoices.set(key, normalizedVoice);
    }
  }

  return Array.from(uniqueVoices.values()).sort((left, right) => {
      if (left.default !== right.default) {
        return left.default ? -1 : 1;
      }

      const leftIsJa = isJapaneseReaderSpeechVoice(left);
      const rightIsJa = isJapaneseReaderSpeechVoice(right);
      if (leftIsJa !== rightIsJa) {
        return leftIsJa ? -1 : 1;
      }

      return left.name.localeCompare(right.name, "ja");
    });
}

class BrowserReaderSpeechEngine implements ReaderSpeechEngine {
  readonly kind = "browser" as const;

  private chunks: ReaderSpeechChunk[] = [];
  private options: ReaderSpeechRequestOptions = {
    rate: DEFAULT_READER_SPEECH_RATE,
    voiceURI: null,
    preferRubyText: true
  };
  private activeUtterance: SpeechSynthesisUtterance | null = null;
  private listeners = new Set<ReaderSpeechEngineListener>();
  private skipEndEvent = false;

  constructor(
    private readonly speechSynthesis: SpeechSynthesis,
    private readonly speechSynthesisUtterance: typeof SpeechSynthesisUtterance
  ) {}

  subscribe(listener: ReaderSpeechEngineListener): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  async prepare(chunks: ReaderSpeechChunk[], options: ReaderSpeechRequestOptions): Promise<void> {
    this.chunks = chunks;
    this.options = {
      ...options,
      rate: clampReaderSpeechRate(options.rate)
    };
    this.emit({ type: "state", requestId: null, state: "preparing" });
    for (const chunk of chunks) {
      this.emit({
        type: "chunkReady",
        requestId: null,
        chunkIndex: chunk.chunkIndex,
        durationMs: chunk.estimatedDurationMs
      });
    }
    this.emit({ type: "state", requestId: null, state: "idle" });
  }

  async play(chunkIndex: number, options: ReaderSpeechPlayOptions = {}): Promise<void> {
    const chunk = this.chunks[chunkIndex];
    if (!chunk) {
      throw new Error("読み上げチャンクが見つかりません。");
    }
    const playback = createReaderSpeechPlaybackChunk(chunk, options.startPosition);

    this.cancelActiveUtterance();
    this.skipEndEvent = false;

    const utterance = new this.speechSynthesisUtterance(playback.chunk.text);
    const availableVoices = this.speechSynthesis.getVoices();
    const selectedVoice =
      availableVoices.find((voice) => voice.voiceURI === this.options.voiceURI) ?? getDefaultReaderSpeechVoice(availableVoices);

    utterance.lang = selectedVoice?.lang || "ja-JP";
    utterance.rate = this.options.rate;
    if (selectedVoice) {
      utterance.voice = selectedVoice;
    }

    utterance.onstart = () => {
      if (this.activeUtterance !== utterance) {
        return;
      }

      this.emit({ type: "state", requestId: null, state: "playing" });
      this.emit({
        type: "progress",
        requestId: null,
        chunkIndex,
        position: playback.chunk.startPosition,
        pageChanged: false,
        source: "start",
        charIndex: playback.charIndexOffset,
        elapsedTimeMs: 0
      });
    };

    utterance.onresume = () => {
      if (this.activeUtterance !== utterance) {
        return;
      }

      this.emit({ type: "state", requestId: null, state: "playing" });
    };

    utterance.onboundary = (event) => {
      if (this.activeUtterance !== utterance || this.skipEndEvent) {
        return;
      }

      this.emit({
        type: "progress",
        requestId: null,
        chunkIndex,
        position: getReaderSpeechPositionForCharIndex(playback.chunk, event.charIndex),
        pageChanged: false,
        source: "boundary",
        charIndex: playback.charIndexOffset + event.charIndex,
        elapsedTimeMs: normalizeSpeechSynthesisElapsedTimeMs(event.elapsedTime)
      });
    };

    utterance.onend = () => {
      if (this.activeUtterance !== utterance) {
        return;
      }

      this.activeUtterance = null;
      if (this.skipEndEvent) {
        return;
      }

      this.emit({
        type: "progress",
        requestId: null,
        chunkIndex,
        position: chunk.endPosition,
        pageChanged: false,
        source: "end",
        charIndex: chunk.text.length,
        elapsedTimeMs: null
      });
      this.emit({
        type: "chunkEnd",
        requestId: null,
        chunkIndex,
        endPosition: chunk.endPosition
      });
    };

    utterance.onerror = (event) => {
      if (this.activeUtterance !== utterance || this.skipEndEvent) {
        return;
      }

      this.activeUtterance = null;
      this.emit({
        type: "error",
        requestId: null,
        chunkIndex,
        message: event.error || "音声読み上げに失敗しました。"
      });
    };

    this.activeUtterance = utterance;
    this.speechSynthesis.speak(utterance);
  }

  async pause(): Promise<void> {
    if (!this.activeUtterance) {
      return;
    }

    this.speechSynthesis.pause();
    this.emit({ type: "state", requestId: null, state: "paused" });
  }

  async resume(): Promise<void> {
    if (!this.activeUtterance) {
      return;
    }

    this.speechSynthesis.resume();
    this.emit({ type: "state", requestId: null, state: "playing" });
  }

  async stop(): Promise<void> {
    this.skipEndEvent = true;
    this.cancelActiveUtterance();
    this.emit({ type: "state", requestId: null, state: "stopped" });
  }

  async dispose(): Promise<void> {
    await this.stop();
    this.listeners.clear();
  }

  private emit(event: ReaderSpeechEngineEvent): void {
    for (const listener of this.listeners) {
      listener(event);
    }
  }

  private cancelActiveUtterance(): void {
    this.activeUtterance = null;
    this.speechSynthesis.cancel();
  }
}

export function createBrowserReaderSpeechEngine(target: ReaderSpeechWindow = window as ReaderSpeechWindow): ReaderSpeechEngine {
  const utteranceConstructor = getSpeechSynthesisUtteranceConstructor(target);
  if (!isBrowserReaderSpeechSupported(target) || !utteranceConstructor) {
    throw new Error("このブラウザでは読み上げを利用できません。");
  }

  return new BrowserReaderSpeechEngine(target.speechSynthesis, utteranceConstructor);
}
