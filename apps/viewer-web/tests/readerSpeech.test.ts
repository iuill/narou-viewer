import { describe, expect, it, vi } from "vitest";
import {
  buildReaderSpeechChunks,
  createBrowserReaderSpeechEngine,
  findReaderSpeechChunkIndex,
  getBrowserReaderSpeechVoices,
  getReaderSpeechCharIndexForPosition,
  getReaderSpeechPositionForCharIndex,
  isBrowserReaderSpeechSupported,
  type ReaderSpeechVoiceOption
} from "../src/readerSpeech";
import type { ReaderDocument } from "../src/readerDocument";

type MockUtteranceLifecycleHandler = () => void;
type MockUtteranceErrorHandler = (event: { error?: string }) => void;
type MockUtteranceBoundaryHandler = (event: { charIndex: number; elapsedTime?: number }) => void;

class MockSpeechSynthesisUtterance {
  lang = "";
  rate = 1;
  voice: SpeechSynthesisVoice | undefined;
  onstart: MockUtteranceLifecycleHandler | null = null;
  onresume: MockUtteranceLifecycleHandler | null = null;
  onboundary: MockUtteranceBoundaryHandler | null = null;
  onend: MockUtteranceLifecycleHandler | null = null;
  onerror: MockUtteranceErrorHandler | null = null;

  constructor(public readonly text: string) {}
}

function createMockVoice(overrides: Partial<SpeechSynthesisVoice> = {}): SpeechSynthesisVoice {
  return {
    default: false,
    lang: "ja-JP",
    localService: true,
    name: "日本語音声",
    voiceURI: "voice:ja",
    ...overrides
  } as SpeechSynthesisVoice;
}

function createMockSpeechSynthesis(voices: SpeechSynthesisVoice[] = [createMockVoice()]) {
  const mock = {
    utterances: [] as MockSpeechSynthesisUtterance[],
    getVoices: vi.fn(() => voices),
    speak: vi.fn((utterance: MockSpeechSynthesisUtterance) => {
      mock.utterances.push(utterance);
    }),
    pause: vi.fn(),
    resume: vi.fn(),
    cancel: vi.fn()
  };

  return mock;
}

let mockSpeechSynthesis = createMockSpeechSynthesis();

describe("readerSpeech", () => {
  it("ルビ優先設定に応じて speech chunk を生成する", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [
            { type: "text", text: "彼は" },
            { type: "ruby", text: "今日", ruby: "きょう" },
            { type: "text", text: "も読む。" }
          ]
        }
      ]
    };

    expect(buildReaderSpeechChunks(readerDocument, { preferRubyText: true })).toEqual([
      expect.objectContaining({
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 8,
        text: "彼はきょうも読む。"
      })
    ]);

    expect(buildReaderSpeechChunks(readerDocument, { preferRubyText: false })).toEqual([
      expect.objectContaining({
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 8,
        text: "彼は今日も読む。"
      })
    ]);
  });

  it("画像を飛ばしつつ html plainText を chunk へ含める", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "title",
          text: "表題"
        },
        {
          type: "image",
          section: "body",
          src: "/cover.png",
          alt: "表紙",
          originalUrl: null,
          title: null
        },
        {
          type: "html",
          section: "body",
          html: "<blockquote>補足</blockquote>",
          plainText: "補足"
        }
      ]
    };

    expect(buildReaderSpeechChunks(readerDocument)).toEqual([
      expect.objectContaining({
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 4,
        text: "表題\n補足"
      })
    ]);
  });

  it("lineBreak を読み上げテキストの改行として保持する", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [
            { type: "text", text: "一行目" },
            { type: "lineBreak" },
            { type: "text", text: "二行目" }
          ]
        }
      ]
    };

    expect(buildReaderSpeechChunks(readerDocument)).toEqual([
      expect.objectContaining({
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 7,
        text: "一行目\n二行目"
      })
    ]);
  });

  it("横棒の連続を表示用加工に関係なく本文どおり読み上げ chunk へ残す", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: "錬金術――――2つ" }]
        }
      ]
    };

    expect(buildReaderSpeechChunks(readerDocument)).toEqual([
      expect.objectContaining({
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 9,
        text: "錬金術――――2つ"
      })
    ]);
  });

  it("長い本文を複数 chunk に分割し開始位置から chunk を引ける", () => {
    const longLine = "あいうえお".repeat(14);
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: longLine }]
        },
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: longLine }]
        }
      ]
    };

    const chunks = buildReaderSpeechChunks(readerDocument, {
      maxChunkGraphemes: 80,
      targetChunkGraphemes: 60
    });

    expect(chunks).toHaveLength(2);
    expect(chunks[0]?.startPosition).toBe(0);
    expect(chunks[0]?.endPosition).toBe(70);
    expect(chunks[1]?.startPosition).toBe(70);
    expect(chunks[1]?.endPosition).toBe(140);
    expect(findReaderSpeechChunkIndex(chunks, 0)).toBe(0);
    expect(findReaderSpeechChunkIndex(chunks, 90)).toBe(1);
    expect(findReaderSpeechChunkIndex(chunks, 999)).toBe(1);
  });

  it("chunk が空のときは開始位置を引けない", () => {
    expect(findReaderSpeechChunkIndex([], 0)).toBeNull();
  });

  it("読み上げ charIndex から chunk 内 position を推定する", () => {
    const chunk = {
      chunkIndex: 0,
      startPosition: 10,
      endPosition: 18,
      text: "あ😀うえ",
      estimatedDurationMs: 900,
      voiceHint: null,
      speakerHint: null
    };

    expect(getReaderSpeechPositionForCharIndex(chunk, 0)).toBe(10);
    expect(getReaderSpeechPositionForCharIndex(chunk, 1)).toBe(12);
    expect(getReaderSpeechPositionForCharIndex(chunk, chunk.text.length)).toBe(18);
    expect(getReaderSpeechCharIndexForPosition(chunk, 10)).toBe(0);
    expect(getReaderSpeechCharIndexForPosition(chunk, 12)).toBe(1);
    expect(getReaderSpeechCharIndexForPosition(chunk, 13)).toBe(3);
    expect(getReaderSpeechCharIndexForPosition(chunk, 18)).toBe(chunk.text.length);
  });

  it("読み上げ position から charIndex を推定するとき境界値を安全に扱う", () => {
    expect(
      getReaderSpeechCharIndexForPosition(
        {
          chunkIndex: 0,
          startPosition: 4,
          endPosition: 12,
          text: "あいうえ",
          estimatedDurationMs: 900,
          voiceHint: null,
          speakerHint: null
        },
        Number.NaN
      )
    ).toBe(0);
    expect(
      getReaderSpeechCharIndexForPosition(
        {
          chunkIndex: 0,
          startPosition: 4,
          endPosition: 4,
          text: "あ",
          estimatedDurationMs: 900,
          voiceHint: null,
          speakerHint: null
        },
        4
      )
    ).toBe(0);
  });

  it("anchor 付き chunk の position から charIndex を推定するとき不連続な境界を安全に扱う", () => {
    const chunk = {
      chunkIndex: 0,
      startPosition: 0,
      endPosition: 10,
      text: "abcdef",
      estimatedDurationMs: 900,
      voiceHint: null,
      speakerHint: null,
      positionAnchors: [
        { charIndex: 2, position: 2 },
        { charIndex: 2, position: 5 },
        { charIndex: 4, position: 7 }
      ]
    };

    expect(getReaderSpeechCharIndexForPosition(chunk, 0)).toBe(2);
    expect(getReaderSpeechCharIndexForPosition(chunk, 4)).toBe(2);
    expect(getReaderSpeechCharIndexForPosition(chunk, 9)).toBe(4);
  });

  it("fragment 境界の anchor を使って読み上げ charIndex を position へ対応させる", () => {
    const readerDocument: ReaderDocument = {
      version: 1,
      blocks: [
        {
          type: "title",
          text: "表題"
        },
        {
          type: "paragraph",
          section: "body",
          inlines: [
            { type: "text", text: "彼は" },
            { type: "ruby", text: "今日", ruby: "きょう" },
            { type: "text", text: "読む。" }
          ]
        }
      ]
    };

    const [chunk] = buildReaderSpeechChunks(readerDocument, { preferRubyText: true });
    expect(chunk).toEqual(
      expect.objectContaining({
        startPosition: 0,
        endPosition: 9,
        text: "表題\n彼はきょう読む。"
      })
    );
    expect(chunk?.positionAnchors).toEqual([
      { charIndex: 0, position: 0 },
      { charIndex: 3, position: 2 },
      { charIndex: 5, position: 4 },
      { charIndex: 8, position: 6 },
      { charIndex: 11, position: 9 }
    ]);
    expect(chunk ? getReaderSpeechPositionForCharIndex(chunk, 3) : null).toBe(2);
    expect(chunk ? getReaderSpeechPositionForCharIndex(chunk, 8) : null).toBe(6);
    expect(chunk ? getReaderSpeechCharIndexForPosition(chunk, 2) : null).toBe(3);
    expect(chunk ? getReaderSpeechCharIndexForPosition(chunk, 6) : null).toBe(8);
  });

  it("利用可能な音声一覧を日本語音声のみに絞って整列する", () => {
    const voices = getBrowserReaderSpeechVoices({
      getVoices() {
        return [
          {
            voiceURI: "voice:en",
            name: "English Voice",
            lang: "en-US",
            default: false,
            localService: true
          },
          {
            voiceURI: "voice:ja",
            name: "Japanese Voice",
            lang: "ja-JP",
            default: false,
            localService: true
          },
          {
            voiceURI: "voice:ja-jp-underscore",
            name: "Japanese Voice 2",
            lang: "ja_JP",
            default: false,
            localService: true
          },
          {
            voiceURI: "voice:ja-jp-variant",
            name: "Japanese Variant",
            lang: "ja-JP-x-private",
            default: false,
            localService: true
          },
          {
            voiceURI: "voice:default",
            name: "Default Voice",
            lang: "en-GB",
            default: true,
            localService: false
          }
        ] as SpeechSynthesisVoice[];
      }
    });

    expect((voices[0] as ReaderSpeechVoiceOption).voiceURI).toBe("voice:ja-jp-variant");
    expect((voices[1] as ReaderSpeechVoiceOption).voiceURI).toBe("voice:ja");
    expect((voices[2] as ReaderSpeechVoiceOption).voiceURI).toBe("voice:ja-jp-underscore");
    expect(voices).toHaveLength(3);
  });

  it("同一音声を重複排除し音声 API がない場合は空配列を返す", () => {
    const duplicateVoice = createMockVoice({ voiceURI: "voice:dup", name: "重複音声" });
    const voices = getBrowserReaderSpeechVoices({
      getVoices() {
        return [duplicateVoice, duplicateVoice];
      }
    });

    expect(voices).toEqual([
      {
        voiceURI: "voice:dup",
        name: "重複音声",
        lang: "ja-JP",
        default: false,
        localService: true
      }
    ]);
    expect(getBrowserReaderSpeechVoices(null)).toEqual([]);
  });

  it("明示 target の読み上げ対応判定を行える", () => {
    const target = {
      speechSynthesis: createMockSpeechSynthesis(),
      SpeechSynthesisUtterance: MockSpeechSynthesisUtterance as unknown as typeof SpeechSynthesisUtterance
    };

    expect(isBrowserReaderSpeechSupported(target)).toBe(true);
    expect(
      isBrowserReaderSpeechSupported({
        speechSynthesis: createMockSpeechSynthesis()
      } as Pick<Window, "speechSynthesis"> as Window)
    ).toBe(false);
  });

  it("browser speech engine が prepare と再生イベントを発火する", async () => {
    const japaneseVoice = createMockVoice({ voiceURI: "voice:ja", name: "標準音声" });
    const fallbackVoice = createMockVoice({
      voiceURI: "voice:fallback",
      name: "既定音声",
      default: true,
      lang: "en-US"
    });
    mockSpeechSynthesis = createMockSpeechSynthesis([fallbackVoice, japaneseVoice]);

    const engine = createBrowserReaderSpeechEngine({
      speechSynthesis: mockSpeechSynthesis as unknown as SpeechSynthesis,
      SpeechSynthesisUtterance: MockSpeechSynthesisUtterance as unknown as typeof SpeechSynthesisUtterance
    });
    const events: unknown[] = [];
    const unsubscribe = engine.subscribe((event) => {
      events.push(event);
    });

    const chunks = [
      {
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 5,
        text: "こんにちは",
        estimatedDurationMs: 900,
        voiceHint: null,
        speakerHint: null
      }
    ];

    await engine.prepare(chunks, { rate: 1.234, voiceURI: "voice:ja", preferRubyText: true });
    await engine.play(0);

    expect(mockSpeechSynthesis.speak).toHaveBeenCalledTimes(1);
    const utterance = mockSpeechSynthesis.utterances[0];
    expect(utterance?.text).toBe("こんにちは");
    expect(utterance?.lang).toBe("ja-JP");
    expect(utterance?.rate).toBe(1.23);
    expect(utterance?.voice).toBe(japaneseVoice);

    utterance?.onstart?.();
    utterance?.onboundary?.({ charIndex: 2, elapsedTime: 0.5 });
    utterance?.onend?.();

    expect(events).toEqual([
      { type: "state", requestId: null, state: "preparing" },
      { type: "chunkReady", requestId: null, chunkIndex: 0, durationMs: 900 },
      { type: "state", requestId: null, state: "idle" },
      { type: "state", requestId: null, state: "playing" },
      {
        type: "progress",
        requestId: null,
        chunkIndex: 0,
        position: 0,
        pageChanged: false,
        source: "start",
        charIndex: 0,
        elapsedTimeMs: 0
      },
      {
        type: "progress",
        requestId: null,
        chunkIndex: 0,
        position: 2,
        pageChanged: false,
        source: "boundary",
        charIndex: 2,
        elapsedTimeMs: 500
      },
      {
        type: "progress",
        requestId: null,
        chunkIndex: 0,
        position: 5,
        pageChanged: false,
        source: "end",
        charIndex: 5,
        elapsedTimeMs: null
      },
      { type: "chunkEnd", requestId: null, chunkIndex: 0, endPosition: 5 }
    ]);

    unsubscribe();
    await engine.dispose();
  });

  it("browser speech engine は開始 position が chunk 途中なら最初の発話を切り出す", async () => {
    mockSpeechSynthesis = createMockSpeechSynthesis();
    const engine = createBrowserReaderSpeechEngine({
      speechSynthesis: mockSpeechSynthesis as unknown as SpeechSynthesis,
      SpeechSynthesisUtterance: MockSpeechSynthesisUtterance as unknown as typeof SpeechSynthesisUtterance
    });
    const events: unknown[] = [];
    engine.subscribe((event) => {
      events.push(event);
    });

    const chunks = [
      {
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 5,
        text: "こんにちは",
        estimatedDurationMs: 900,
        voiceHint: null,
        speakerHint: null
      }
    ];

    await engine.prepare(chunks, { rate: 1, voiceURI: null, preferRubyText: true });
    await engine.play(0, { startPosition: 2 });

    const utterance = mockSpeechSynthesis.utterances[0];
    expect(utterance?.text).toBe("にちは");

    utterance?.onstart?.();
    utterance?.onboundary?.({ charIndex: 1, elapsedTime: 0.25 });
    utterance?.onend?.();

    expect(events).toContainEqual({
      type: "progress",
      requestId: null,
      chunkIndex: 0,
      position: 2,
      pageChanged: false,
      source: "start",
      charIndex: 2,
      elapsedTimeMs: 0
    });
    expect(events).toContainEqual({
      type: "progress",
      requestId: null,
      chunkIndex: 0,
      position: 3,
      pageChanged: false,
      source: "boundary",
      charIndex: 3,
      elapsedTimeMs: 250
    });
    expect(events).toContainEqual({
      type: "progress",
      requestId: null,
      chunkIndex: 0,
      position: 5,
      pageChanged: false,
      source: "end",
      charIndex: 5,
      elapsedTimeMs: null
    });

    await engine.dispose();
  });

  it("browser speech engine は anchor 付き chunk の途中再生でも元 chunk 基準の charIndex を通知する", async () => {
    mockSpeechSynthesis = createMockSpeechSynthesis();
    const engine = createBrowserReaderSpeechEngine({
      speechSynthesis: mockSpeechSynthesis as unknown as SpeechSynthesis,
      SpeechSynthesisUtterance: MockSpeechSynthesisUtterance as unknown as typeof SpeechSynthesisUtterance
    });
    const events: unknown[] = [];
    engine.subscribe((event) => {
      events.push(event);
    });

    const chunks = [
      {
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 9,
        text: "表題\n彼はきょう読む。",
        estimatedDurationMs: 900,
        voiceHint: null,
        speakerHint: null,
        positionAnchors: [
          { charIndex: 0, position: 0 },
          { charIndex: 3, position: 2 },
          { charIndex: 5, position: 4 },
          { charIndex: 8, position: 6 },
          { charIndex: 11, position: 9 }
        ]
      }
    ];

    await engine.prepare(chunks, { rate: 1, voiceURI: null, preferRubyText: true });
    await engine.play(0, { startPosition: 4 });

    const utterance = mockSpeechSynthesis.utterances[0];
    expect(utterance?.text).toBe("きょう読む。");

    utterance?.onstart?.();
    utterance?.onboundary?.({ charIndex: 3, elapsedTime: 0.75 });

    expect(events).toContainEqual({
      type: "progress",
      requestId: null,
      chunkIndex: 0,
      position: 4,
      pageChanged: false,
      source: "start",
      charIndex: 5,
      elapsedTimeMs: 0
    });
    expect(events).toContainEqual({
      type: "progress",
      requestId: null,
      chunkIndex: 0,
      position: 6,
      pageChanged: false,
      source: "boundary",
      charIndex: 8,
      elapsedTimeMs: 750
    });

    await engine.dispose();
  });

  it("browser speech engine は開始位置が末尾文字に丸められる場合は chunk 先頭から再生する", async () => {
    mockSpeechSynthesis = createMockSpeechSynthesis();
    const engine = createBrowserReaderSpeechEngine({
      speechSynthesis: mockSpeechSynthesis as unknown as SpeechSynthesis,
      SpeechSynthesisUtterance: MockSpeechSynthesisUtterance as unknown as typeof SpeechSynthesisUtterance
    });

    const chunks = [
      {
        chunkIndex: 0,
        startPosition: 0,
        endPosition: 2,
        text: "あ",
        estimatedDurationMs: 90,
        voiceHint: null,
        speakerHint: null
      }
    ];

    await engine.prepare(chunks, { rate: 1, voiceURI: null, preferRubyText: true });
    await engine.play(0, { startPosition: 1 });

    expect(mockSpeechSynthesis.utterances[0]?.text).toBe("あ");

    await engine.dispose();
  });

  it("browser speech engine が pause resume stop error を扱う", async () => {
    mockSpeechSynthesis = createMockSpeechSynthesis();
    const engine = createBrowserReaderSpeechEngine({
      speechSynthesis: mockSpeechSynthesis as unknown as SpeechSynthesis,
      SpeechSynthesisUtterance: MockSpeechSynthesisUtterance as unknown as typeof SpeechSynthesisUtterance
    });
    const events: unknown[] = [];
    engine.subscribe((event) => {
      events.push(event);
    });

    const chunks = [
      {
        chunkIndex: 0,
        startPosition: 3,
        endPosition: 8,
        text: "テストです",
        estimatedDurationMs: 810,
        voiceHint: null,
        speakerHint: null
      }
    ];

    await engine.prepare(chunks, { rate: 0.1, voiceURI: "missing", preferRubyText: true });
    await engine.play(0);
    const utterance = mockSpeechSynthesis.utterances[0];

    await engine.pause();
    await engine.resume();
    utterance?.onresume?.();
    utterance?.onerror?.({});

    expect(mockSpeechSynthesis.pause).toHaveBeenCalledTimes(1);
    expect(mockSpeechSynthesis.resume).toHaveBeenCalledTimes(1);
    expect(events).toContainEqual({ type: "state", requestId: null, state: "paused" });
    expect(events).toContainEqual({ type: "state", requestId: null, state: "playing" });
    expect(events).toContainEqual({
      type: "error",
      requestId: null,
      chunkIndex: 0,
      message: "音声読み上げに失敗しました。"
    });

    await engine.play(0);
    const utteranceToStop = mockSpeechSynthesis.utterances[1];
    await engine.stop();
    utteranceToStop?.onend?.();

    expect(mockSpeechSynthesis.cancel).toHaveBeenCalled();
    expect(events).toContainEqual({ type: "state", requestId: null, state: "stopped" });
    expect(events).not.toContainEqual({ type: "chunkEnd", requestId: null, chunkIndex: 0, endPosition: 8 });
  });

  it("browser speech engine は不正な chunk index や非対応 target を弾く", async () => {
    mockSpeechSynthesis = createMockSpeechSynthesis();
    const engine = createBrowserReaderSpeechEngine({
      speechSynthesis: mockSpeechSynthesis as unknown as SpeechSynthesis,
      SpeechSynthesisUtterance: MockSpeechSynthesisUtterance as unknown as typeof SpeechSynthesisUtterance
    });

    await engine.prepare([], { rate: 1, voiceURI: null, preferRubyText: true });

    await expect(engine.play(0)).rejects.toThrow("読み上げチャンクが見つかりません。");
    expect(() =>
      createBrowserReaderSpeechEngine({
        speechSynthesis: mockSpeechSynthesis as unknown as SpeechSynthesis
      } as Pick<Window, "speechSynthesis"> as Window)
    ).toThrow("このブラウザでは読み上げを利用できません。");
  });
});
