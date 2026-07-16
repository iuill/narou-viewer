import { describe, expect, it, vi } from "vitest";
import { JSDOM } from "jsdom";
import { formatDate, formatElapsedMs } from "../src/shared/date";
import {
  compareEpisodeIndex,
  getPreviousEpisodeIndex,
  normalizeEpisodeIndex
} from "../src/features/reader/episodeIndex";
import {
  exitDocumentFullscreen,
  getFullscreenElement,
  requestElementFullscreen,
  resolveReaderFullscreenToggleAction,
  shouldExitReaderFullscreenOnReturn,
  supportsElementFullscreen
} from "../src/features/reader/fullscreen";
import {
  canMoveReaderPage,
  getReaderEdgeTapPageMoveDirections,
  getReaderPageMoveDirections,
  getReaderTouchGestureStart,
  hasActiveTextSelection,
  resolveReaderEdgeNavigationDirection,
  resolveReaderTouchNavigationDirection
} from "../src/features/reader/gestureNavigation";
import { createReadingStateKey } from "../src/features/reader/readerStateKey";
import { getReaderControlCapacity } from "../src/features/reader/controlsLayout";
import {
  E2E_CONTROL_WINDOW_KEY,
  getE2EControlStore,
  isReaderStateSaveDisabled
} from "../src/testing/e2eControl";
import {
  createAiGenerationProfileDraft,
  createAiGenerationPlaygroundInitialProgress,
  formatAiGenerationPlaygroundError,
  getAiGenerationApiKeyStatusLabel,
  getActiveAiGenerationSettingsProfile,
  getCompactAiGenerationModelLabel,
  getAiGenerationModeLabel,
  getAiGenerationSummary,
  getAiGenerationTriggerSummary,
  getVisibleAiGenerationJobs,
  partitionAiGenerationJobs,
  removeAiGenerationProfileDraft,
  resolveAiGenerationProfileDraftSelection,
  toAiGenerationProfileDraft,
  type AiGenerationJobLike,
  type AiGenerationProfileDraft
} from "../src/features/ai-generation/model";

describe("frontend helper modules", () => {
  it("creates stable reading state keys only when all values are present", () => {
    expect(createReadingStateKey("novel-a", "12", 34)).toBe("novel-a:12:34");
    expect(createReadingStateKey(null, "12", 34)).toBeNull();
    expect(createReadingStateKey("novel-a", null, 34)).toBeNull();
    expect(createReadingStateKey("novel-a", "12", null)).toBeNull();
  });

  it("reads the E2E control store safely", () => {
    expect(getE2EControlStore(undefined)).toBeNull();
    expect(getE2EControlStore({ [E2E_CONTROL_WINDOW_KEY]: "invalid" } as never)).toBeNull();
    expect(getE2EControlStore({ [E2E_CONTROL_WINDOW_KEY]: { disableReaderStateSave: true } } as never)).toEqual({
      disableReaderStateSave: true
    });
  });

  it("detects whether reader state saving is disabled", () => {
    expect(isReaderStateSaveDisabled(undefined)).toBe(false);
    expect(isReaderStateSaveDisabled({ [E2E_CONTROL_WINDOW_KEY]: { disableReaderStateSave: false } } as never)).toBe(
      false
    );
    expect(isReaderStateSaveDisabled({ [E2E_CONTROL_WINDOW_KEY]: { disableReaderStateSave: true } } as never)).toBe(
      true
    );
  });

  it("resolves fullscreen elements from standard and webkit APIs", () => {
    const element = {} as Element;
    expect(getFullscreenElement(undefined)).toBeNull();
    expect(getFullscreenElement({ fullscreenElement: element } as never)).toBe(element);
    expect(getFullscreenElement({ webkitFullscreenElement: element } as never)).toBe(element);
  });

  it("detects fullscreen capability and requests fullscreen using available APIs", async () => {
    const requestFullscreen = vi.fn().mockResolvedValue(undefined);
    const webkitRequestFullscreen = vi.fn().mockResolvedValue(undefined);

    expect(supportsElementFullscreen({ requestFullscreen } as never)).toBe(true);
    expect(supportsElementFullscreen({ webkitRequestFullscreen } as never)).toBe(true);
    expect(supportsElementFullscreen({} as never)).toBe(false);

    await requestElementFullscreen({ requestFullscreen } as never);
    await requestElementFullscreen({ webkitRequestFullscreen } as never);
    await expect(requestElementFullscreen({} as never)).rejects.toThrow("Fullscreen API is not available.");

    expect(requestFullscreen).toHaveBeenCalledTimes(1);
    expect(webkitRequestFullscreen).toHaveBeenCalledTimes(1);
  });

  it("exits fullscreen using available APIs and no-ops when absent", async () => {
    const exitFullscreen = vi.fn().mockResolvedValue(undefined);
    const webkitExitFullscreen = vi.fn().mockResolvedValue(undefined);

    await exitDocumentFullscreen({ exitFullscreen } as never);
    await exitDocumentFullscreen({ webkitExitFullscreen } as never);
    await expect(exitDocumentFullscreen(undefined)).resolves.toBeUndefined();

    expect(exitFullscreen).toHaveBeenCalledTimes(1);
    expect(webkitExitFullscreen).toHaveBeenCalledTimes(1);
  });

  it("formats dates, durations, and episode indexes", () => {
    expect(formatDate(null)).toBe("未取得");
    expect(formatDate("not-a-date")).toBe("not-a-date");
    expect(formatDate("2026-03-12T10:15:00.000Z")).toContain("2026");

    expect(formatElapsedMs(320)).toBe("320ms");
    expect(formatElapsedMs(1500)).toBe("1.50s");
    expect(formatElapsedMs(12_000)).toBe("12.0s");

    expect(getPreviousEpisodeIndex("2")).toBe("1");
    expect(getPreviousEpisodeIndex("1")).toBeNull();
    expect(getPreviousEpisodeIndex("x")).toBeNull();

    expect(compareEpisodeIndex("10", "2")).toBe(8);

    expect(normalizeEpisodeIndex("12")).toBe("12");
    expect(normalizeEpisodeIndex(0)).toBe("0");
    expect(normalizeEpisodeIndex(-1)).toBeNull();
    expect(normalizeEpisodeIndex(1.5)).toBeNull();
    expect(normalizeEpisodeIndex("01a")).toBeNull();
  });

  it("calculates reader control capacity and paging behavior", () => {
    expect(getReaderControlCapacity(120, false)).toBe(4);
    expect(getReaderControlCapacity(400, false)).toBe(7);
    expect(getReaderControlCapacity(400, true, 100)).toBe(5);
    expect(getReaderControlCapacity(400, false, 0, 52)).toBe(6);

    expect(getReaderPageMoveDirections("vertical")).toEqual({ previous: 1, next: -1 });
    expect(getReaderPageMoveDirections("horizontal")).toEqual({ previous: -1, next: 1 });
    expect(getReaderEdgeTapPageMoveDirections({ previous: -1, next: 1 }, false)).toEqual({ previous: -1, next: 1 });
    expect(getReaderEdgeTapPageMoveDirections({ previous: -1, next: 1 }, true)).toEqual({ previous: 1, next: -1 });

    expect(canMoveReaderPage(0, 5, -1)).toBe(false);
    expect(canMoveReaderPage(0, 5, 1)).toBe(true);
    expect(canMoveReaderPage(4, 5, 1)).toBe(false);
  });

  it("resolves touch gestures, edge navigation, and fullscreen actions", () => {
    expect(
      getReaderTouchGestureStart({
        firstTouch: { clientX: 100, clientY: 120 },
        nowMs: 1234,
        touchCount: 1
      })
    ).toEqual({
      startClientX: 100,
      startClientY: 120,
      startTimeMs: 1234
    });
    expect(
      getReaderTouchGestureStart({
        firstTouch: { clientX: 100, clientY: 120 },
        touchCount: 2
      })
    ).toBeNull();

    expect(
      resolveReaderEdgeNavigationDirection({
        viewportLeft: 10,
        viewportWidth: 300,
        clientX: 20,
        pageMoveDirections: { previous: -1, next: 1 }
      })
    ).toBe(-1);
    expect(
      resolveReaderEdgeNavigationDirection({
        viewportLeft: 10,
        viewportWidth: 300,
        clientX: 295,
        pageMoveDirections: { previous: -1, next: 1 }
      })
    ).toBe(1);
    expect(
      resolveReaderEdgeNavigationDirection({
        viewportLeft: 10,
        viewportWidth: 300,
        clientX: 160,
        pageMoveDirections: { previous: -1, next: 1 }
      })
    ).toBeNull();

    expect(
      resolveReaderTouchNavigationDirection({
        touchGesture: { startClientX: 100, startClientY: 120, startTimeMs: 1000 },
        endTouch: { clientX: 180, clientY: 122 },
        isTextSelectionGesture: true,
        nowMs: 1600,
        viewportLeft: 10,
        viewportWidth: 300,
        swipePageMoveDirections: { previous: -1, next: 1 },
        tapPageMoveDirections: { previous: 1, next: -1 }
      })
    ).toBeNull();
    expect(
      resolveReaderTouchNavigationDirection({
        touchGesture: { startClientX: 100, startClientY: 120, startTimeMs: 1000 },
        endTouch: { clientX: 180, clientY: 122 },
        nowMs: 1100,
        viewportLeft: 10,
        viewportWidth: 300,
        swipePageMoveDirections: { previous: -1, next: 1 },
        tapPageMoveDirections: { previous: 1, next: -1 }
      })
    ).toBe(-1);
    expect(
      resolveReaderTouchNavigationDirection({
        touchGesture: { startClientX: 100, startClientY: 120, startTimeMs: 1000 },
        endTouch: { clientX: 20, clientY: 120 },
        nowMs: 1600,
        viewportLeft: 10,
        viewportWidth: 300,
        swipePageMoveDirections: { previous: -1, next: 1 },
        tapPageMoveDirections: { previous: 1, next: -1 }
      })
    ).toBe(1);
    expect(
      resolveReaderTouchNavigationDirection({
        touchGesture: { startClientX: 20, startClientY: 120, startTimeMs: 1000 },
        endTouch: { clientX: 20, clientY: 120 },
        nowMs: 1100,
        viewportLeft: 10,
        viewportWidth: 300,
        swipePageMoveDirections: { previous: -1, next: 1 },
        tapPageMoveDirections: { previous: 1, next: -1 }
      })
    ).toBe(1);
    expect(
      resolveReaderTouchNavigationDirection({
        touchGesture: { startClientX: 20, startClientY: 120, startTimeMs: 1000 },
        endTouch: { clientX: 20, clientY: 120 },
        nowMs: 1600,
        viewportLeft: 10,
        viewportWidth: 300,
        swipePageMoveDirections: { previous: -1, next: 1 },
        tapPageMoveDirections: { previous: 1, next: -1 }
      })
    ).toBeNull();
    expect(
      resolveReaderTouchNavigationDirection({
        touchGesture: null,
        endTouch: { clientX: 295, clientY: 120 },
        viewportLeft: 10,
        viewportWidth: 300,
        swipePageMoveDirections: { previous: -1, next: 1 },
        tapPageMoveDirections: { previous: 1, next: -1 }
      })
    ).toBeNull();
    expect(
      resolveReaderTouchNavigationDirection({
        touchGesture: null,
        endTouch: null,
        viewportLeft: 10,
        viewportWidth: 300,
        swipePageMoveDirections: { previous: -1, next: 1 },
        tapPageMoveDirections: { previous: 1, next: -1 }
      })
    ).toBeNull();

    expect(
      resolveReaderFullscreenToggleAction({
        hasReaderShell: false,
        isNativeFullscreen: false,
        isPseudoFullscreen: false,
        supportsNativeFullscreen: true
      })
    ).toBe("noop");
    expect(
      resolveReaderFullscreenToggleAction({
        hasReaderShell: true,
        isNativeFullscreen: true,
        isPseudoFullscreen: false,
        supportsNativeFullscreen: true
      })
    ).toBe("exit-native");
    expect(
      resolveReaderFullscreenToggleAction({
        hasReaderShell: true,
        isNativeFullscreen: false,
        isPseudoFullscreen: true,
        supportsNativeFullscreen: true
      })
    ).toBe("disable-pseudo");
    expect(
      resolveReaderFullscreenToggleAction({
        hasReaderShell: true,
        isNativeFullscreen: false,
        isPseudoFullscreen: false,
        supportsNativeFullscreen: false
      })
    ).toBe("enable-pseudo");
    expect(
      resolveReaderFullscreenToggleAction({
        hasReaderShell: true,
        isNativeFullscreen: false,
        isPseudoFullscreen: false,
        supportsNativeFullscreen: true
      })
    ).toBe("request-native");

    expect(
      shouldExitReaderFullscreenOnReturn({
        hasReaderShell: true,
        isNativeFullscreen: true
      })
    ).toBe(true);
    expect(
      shouldExitReaderFullscreenOnReturn({
        hasReaderShell: false,
        isNativeFullscreen: true
      })
    ).toBe(false);
  });

  it("detects active text selection inside the reader viewport", () => {
    const dom = new JSDOM(`<div id="reader"><p>本文テキスト</p></div><div id="outside">外</div>`);
    const reader = dom.window.document.getElementById("reader");
    const outside = dom.window.document.getElementById("outside");
    const textNode = reader?.querySelector("p")?.firstChild;
    const selection = dom.window.getSelection();

    expect(hasActiveTextSelection(null, reader)).toBe(false);
    expect(hasActiveTextSelection(selection as unknown as Selection, reader)).toBe(false);
    if (!reader || !outside || !textNode || !selection) {
      throw new Error("selection fixture was not created");
    }

    const range = dom.window.document.createRange();
    range.setStart(textNode, 0);
    range.setEnd(textNode, 2);
    selection.removeAllRanges();
    selection.addRange(range);

    expect(hasActiveTextSelection(selection as unknown as Selection, reader)).toBe(true);
    expect(hasActiveTextSelection(selection as unknown as Selection, outside)).toBe(false);
    expect(hasActiveTextSelection(selection as unknown as Selection)).toBe(true);

    selection.removeAllRanges();
    expect(hasActiveTextSelection(selection as unknown as Selection, reader)).toBe(false);
  });

  it("formats AI generation mode labels and summaries", () => {
    expect(getAiGenerationModeLabel("openrouter")).toBe("OpenRouter");
    expect(getAiGenerationModeLabel("heuristic")).toBe("Heuristic");
    expect(getAiGenerationModeLabel("disabled")).toBe("停止");
    expect(getAiGenerationModeLabel(null)).toBe("未記録");
    expect(getCompactAiGenerationModelLabel("openai/gpt-5-mini")).toBe("gpt-5-mini");
    expect(getCompactAiGenerationModelLabel("gpt-5-mini")).toBe("gpt-5-mini");
    expect(getCompactAiGenerationModelLabel("")).toBeNull();
    expect(getCompactAiGenerationModelLabel(null)).toBeNull();
    expect(getCompactAiGenerationModelLabel("/")).toBeNull();
    expect(getCompactAiGenerationModelLabel("///")).toBeNull();
    expect(getCompactAiGenerationModelLabel("   /   /   ")).toBeNull();
    expect(getCompactAiGenerationModelLabel("   ")).toBeNull();
    expect(getCompactAiGenerationModelLabel("  openai/gpt-5-mini  ")).toBe("gpt-5-mini");

    expect(
      getAiGenerationSummary(
        {
          effectiveGenerationMode: "openrouter",
          settings: {
            selectedProfileId: "profile-b",
            profiles: [
              {
                id: "profile-a",
                label: "A",
                hasApiKey: true,
                apiKeyMasked: "sk-***",
                modelId: "openai/gpt-5-mini",
                providerOrder: ["openai"],
                allowFallbacks: true,
                requireParameters: true
              },
              {
                id: "profile-b",
                label: "B",
                hasApiKey: true,
                apiKeyMasked: "sk-***",
                modelId: "anthropic/claude-sonnet-4",
                providerOrder: ["anthropic"],
                allowFallbacks: false,
                requireParameters: true
              }
            ]
          }
        },
        null
      )
    ).toBe("OpenRouter / anthropic/claude-sonnet-4");

    expect(
      getAiGenerationTriggerSummary(
        {
          effectiveGenerationMode: "openrouter",
          settings: {
            selectedProfileId: "profile-b",
            profiles: [
              {
                id: "profile-a",
                label: "A",
                hasApiKey: true,
                apiKeyMasked: "sk-***",
                modelId: "openai/gpt-5-mini",
                providerOrder: ["openai"],
                allowFallbacks: true,
                requireParameters: true
              },
              {
                id: "profile-b",
                label: "B",
                hasApiKey: true,
                apiKeyMasked: "sk-***",
                modelId: "anthropic/claude-sonnet-4",
                providerOrder: ["anthropic"],
                allowFallbacks: false,
                requireParameters: true
              }
            ]
          }
        },
        null
      )
    ).toBe("OpenRouter / claude-sonnet-4");

    expect(
      getAiGenerationSummary(
        {
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: null,
            profiles: [
              {
                id: "profile-a",
                label: "A",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: false,
                requireParameters: true
              }
            ]
          }
        },
        null
      )
    ).toBe("Heuristic");

    expect(
      getAiGenerationTriggerSummary(
        {
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: null,
            profiles: [
              {
                id: "profile-a",
                label: "A",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: false,
                requireParameters: true
              }
            ]
          }
        },
        null
      )
    ).toBe("Heuristic");

    expect(
      getAiGenerationSummary(null, {
        services: [
          { id: "viewer-api", summary: "ok" },
          { id: "go-internal-ai", summary: "degraded" }
        ]
      })
    ).toBe("degraded");
    expect(getAiGenerationTriggerSummary(null, { services: [{ id: "go-internal-ai", summary: "degraded" }] })).toBe(
      "degraded"
    );

    expect(getAiGenerationSummary(null, null)).toBe("確認中");
    expect(getAiGenerationTriggerSummary(null, null)).toBe("確認中");
  });

  it("resolves the active settings profile and draft selections", () => {
    expect(getActiveAiGenerationSettingsProfile(null)).toBeNull();

    expect(
      getActiveAiGenerationSettingsProfile({
        effectiveGenerationMode: "openrouter",
        settings: {
          selectedProfileId: "profile-b",
          profiles: [
            {
              id: "profile-a",
              label: "A",
              hasApiKey: true,
              apiKeyMasked: "sk-***",
              modelId: "openai/gpt-5-mini",
              providerOrder: ["openai"],
              allowFallbacks: true,
              requireParameters: true
            },
            {
              id: "profile-b",
              label: "B",
              hasApiKey: false,
              apiKeyMasked: null,
              modelId: null,
              providerOrder: [],
              allowFallbacks: false,
              requireParameters: true
            }
          ]
        }
      })
    ).toEqual(expect.objectContaining({ id: "profile-b" }));

    const profiles: AiGenerationProfileDraft[] = [
      {
        id: "profile-a",
        label: "A",
        apiKeyInput: "",
        apiKeyMasked: "sk-***",
        hasApiKey: true,
        modelId: "openai/gpt-5-mini",
        providerOrder: "openai",
        allowFallbacks: true,
        requireParameters: true
      },
      {
        id: "profile-b",
        label: "B",
        apiKeyInput: "",
        apiKeyMasked: null,
        hasApiKey: false,
        modelId: "",
        providerOrder: "",
        allowFallbacks: false,
        requireParameters: true
      }
    ];

    expect(
      resolveAiGenerationProfileDraftSelection({
        profiles,
        selectedProfileId: "profile-b",
        editingProfileId: "missing",
        playgroundProfileId: "profile-a"
      })
    ).toEqual({
      defaultProfile: profiles[1],
      editingProfile: profiles[1],
      playgroundProfile: profiles[0]
    });
  });

  it("normalizes AI generation profile drafts and API key labels", () => {
    const draft = toAiGenerationProfileDraft({
      id: "profile-a",
      label: "Profile A",
      provider: "openrouter",
      credentials: {
        source: "custom",
        hasApiKey: true,
        apiKeyMasked: "sk-***",
        updatedAt: "2026-03-22T10:00:00Z"
      },
      modelId: "openai/gpt-5-mini",
      providerOrder: ["openai", "anthropic"],
      allowFallbacks: true,
      requireParameters: false
    });

    expect(draft).toEqual({
      id: "profile-a",
      label: "Profile A",
      provider: "openrouter",
      apiKeySource: "custom",
      apiKeyInput: "",
      apiKeyMasked: "sk-***",
      hasApiKey: true,
      credentialsUpdatedAt: "2026-03-22T10:00:00Z",
      modelId: "openai/gpt-5-mini",
      providerOrder: "openai, anthropic",
      allowFallbacks: true,
      requireParameters: false
    });

    expect(getAiGenerationApiKeyStatusLabel(draft)).toBe("sk-***");
    expect(
      getAiGenerationApiKeyStatusLabel({
        ...draft,
        apiKeyMasked: null
      })
    ).toBe("保存済み(復号不可)");

    const emptyDraft: AiGenerationProfileDraft = {
      ...draft,
      apiKeyMasked: null,
      hasApiKey: false
    };
    expect(getAiGenerationApiKeyStatusLabel(emptyDraft)).toBe("未設定");
  });

  it("creates, partitions, filters, and removes AI generation profile and job state", () => {
    expect(createAiGenerationProfileDraft(2, () => "profile-new")).toEqual({
      id: "profile-new",
      label: "Profile 3",
      provider: "openrouter",
      apiKeySource: "shared",
      apiKeyInput: "",
      apiKeyMasked: null,
      hasApiKey: false,
      credentialsUpdatedAt: null,
      modelId: "",
      providerOrder: "",
      allowFallbacks: false,
      requireParameters: true
    });

    const jobs: AiGenerationJobLike[] = [
      { status: "queued" },
      { status: "running" },
      { status: "failed" },
      { status: "incompatible" },
      { status: "completed" }
    ];

    expect(partitionAiGenerationJobs(jobs)).toEqual({
      active: [{ status: "queued" }, { status: "running" }],
      failed: [{ status: "failed" }, { status: "incompatible" }],
      completed: [{ status: "completed" }]
    });

    expect(getVisibleAiGenerationJobs({ jobs, filter: "active" })).toEqual([{ status: "queued" }, { status: "running" }]);
    expect(getVisibleAiGenerationJobs({ jobs, filter: "failed" })).toEqual([
      { status: "failed" },
      { status: "incompatible" }
    ]);
    expect(getVisibleAiGenerationJobs({ jobs, filter: "completed" })).toEqual([{ status: "completed" }]);

    expect(
      removeAiGenerationProfileDraft({
        profiles: [
          {
            id: "profile-a",
            label: "A",
            apiKeyInput: "",
            apiKeyMasked: null,
            hasApiKey: false,
            modelId: "",
            providerOrder: "",
            allowFallbacks: false,
            requireParameters: true
          },
          {
            id: "profile-b",
            label: "B",
            apiKeyInput: "",
            apiKeyMasked: null,
            hasApiKey: false,
            modelId: "",
            providerOrder: "",
            allowFallbacks: false,
            requireParameters: true
          }
        ],
        profileId: "profile-a",
        selectedProfileId: "profile-a",
        editingProfileId: "profile-b",
        playgroundProfileId: "profile-a"
      })
    ).toEqual({
      profiles: [
        {
          id: "profile-b",
          label: "B",
          apiKeyInput: "",
          apiKeyMasked: null,
          hasApiKey: false,
          modelId: "",
          providerOrder: "",
          allowFallbacks: false,
          requireParameters: true
        }
      ],
      selectedProfileId: "profile-b",
      editingProfileId: "profile-b",
      playgroundProfileId: "profile-b"
    });
  });

  it("creates initial playground progress", () => {
    expect(createAiGenerationPlaygroundInitialProgress()).toEqual({
      stage: "preparing",
      message: "進捗ストリームを開始しています。",
      progress: 5,
      step: 0,
      stepCount: 4
    });
  });


  it("formats playground errors by likely cause", () => {
    expect(formatAiGenerationPlaygroundError("selected profile was not found")).toContain("プロファイルが見つかりません");
    expect(formatAiGenerationPlaygroundError("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE is missing")).toContain(
      "復号に必要です"
    );
    expect(formatAiGenerationPlaygroundError("service not configured")).toContain("ヒューリスティックに切り替える");
    expect(formatAiGenerationPlaygroundError("OpenRouter API key is invalid")).toContain("APIキー と モデル設定");
    expect(formatAiGenerationPlaygroundError("unexpected failure")).toBe(
      "生成テストの実行に失敗しました。unexpected failure"
    );
  });
});
