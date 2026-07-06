import { describe, expect, it, vi } from "vitest";

import { ApiResponseError } from "../src/api/errors";
import { createApiHeaders } from "../src/api/http";
import {
  postAiGenerationPlaygroundStream,
  readAiGenerationPlaygroundStream
} from "../src/features/ai-generation/api";
import {
  deleteBookmark,
  postReaderAiAssistantChatStream,
  putReaderState,
  readReaderAiAssistantStream
} from "../src/features/reader/api";
import { fetchRuntimeStatus } from "../src/features/runtime/api";
import { API_CLIENT_UPDATE_REQUIRED_EVENT } from "../src/api/contract";

function createNdjsonResponse(chunks: string[]): Response {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      }
    }),
    {
      status: 200,
      headers: {
        "content-type": "application/x-ndjson"
      }
    }
  );
}

describe("api client stream readers", () => {
  it("adds API contract and client build headers to common requests", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ runtime: "ok" }), {
        status: 200,
        headers: {
          "content-type": "application/json"
        }
      })
    );
    vi.stubGlobal("fetch", fetchMock);

    await fetchRuntimeStatus();

    const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
    const headers = init.headers as Headers;
    expect(headers.get("x-narou-viewer-api-contract-version")).toBe("1");
    expect(headers.get("x-narou-viewer-client-build")).toBeTruthy();

    vi.unstubAllGlobals();
  });

  it("preserves structured API error metadata while keeping the user-facing message", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: "Client update required.",
          code: "CLIENT_UPDATE_REQUIRED",
          message: "Client update required.",
          details: { minApiContractVersion: "1" },
          requestId: "api-request-1"
        }),
        {
          status: 426,
          headers: {
            "content-type": "application/json",
            "x-narou-viewer-reload-required": "1"
          }
        }
      )
    );
    vi.stubGlobal("fetch", fetchMock);

    try {
      await fetchRuntimeStatus();
      throw new Error("fetchRuntimeStatus should reject");
    } catch (error) {
      expect(error).toBeInstanceOf(ApiResponseError);
      expect(error).toMatchObject({
        message: "Client update required.",
        status: 426,
        code: "CLIENT_UPDATE_REQUIRED",
        requestId: "api-request-1",
        reloadRequired: true
      });
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("dispatches a client update event for reload-required API errors", async () => {
    const eventTarget = new EventTarget();
    const listener = vi.fn();
    eventTarget.addEventListener(API_CLIENT_UPDATE_REQUIRED_EVENT, listener);
    vi.stubGlobal("window", eventTarget);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: "Client update required.",
            code: "CLIENT_UPDATE_REQUIRED",
            message: "Client update required.",
            details: { minApiContractVersion: "1" }
          }),
          {
            status: 426,
            headers: {
              "content-type": "application/json",
              "x-narou-viewer-reload-required": "1"
            }
          }
        )
      )
    );

    await expect(fetchRuntimeStatus()).rejects.toBeInstanceOf(ApiResponseError);

    expect(listener).toHaveBeenCalledTimes(1);
    expect((listener.mock.calls[0]?.[0] as CustomEvent).detail).toMatchObject({
      code: "CLIENT_UPDATE_REQUIRED",
      minApiContractVersion: "1",
      status: 426
    });

    vi.unstubAllGlobals();
  });

  it("dispatches a single client update event for Playground stream errors", async () => {
    const eventTarget = new EventTarget();
    const listener = vi.fn();
    eventTarget.addEventListener(API_CLIENT_UPDATE_REQUIRED_EVENT, listener);
    vi.stubGlobal("window", eventTarget);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: "Client update required.",
            code: "CLIENT_UPDATE_REQUIRED",
            message: "Client update required.",
            details: { minApiContractVersion: "1" }
          }),
          {
            status: 426,
            headers: {
              "content-type": "application/json",
              "x-narou-viewer-reload-required": "1"
            }
          }
        )
      )
    );

    await expect(
      postAiGenerationPlaygroundStream({
        novelId: "novel-a",
        upToEpisodeIndex: "1"
      })
    ).rejects.toMatchObject({
      name: "ApiResponseError",
      code: "CLIENT_UPDATE_REQUIRED"
    });

    expect(listener).toHaveBeenCalledTimes(1);

    vi.unstubAllGlobals();
  });

  it("does not treat update-required reader state responses as sync conflicts", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            error: "Client update required.",
            code: "CLIENT_UPDATE_REQUIRED",
            message: "Client update required."
          }),
          {
            status: 409,
            headers: {
              "content-type": "application/json",
              "x-narou-viewer-reload-required": "1"
            }
          }
        )
      )
    );

    await expect(
      putReaderState({
        novelId: "novel-a",
        lastReadEpisodeIndex: "1",
        position: 10,
        scroll: null,
        clientId: "client-a",
        expectedStateVersion: 1
      })
    ).rejects.toMatchObject({
      name: "ApiResponseError",
      code: "CLIENT_UPDATE_REQUIRED"
    });

    vi.unstubAllGlobals();
  });

  it("preserves structured errors for non-common response paths", async () => {
    const errorResponse = () =>
      new Response(
        JSON.stringify({
          error: "Client update required.",
          code: "CLIENT_UPDATE_REQUIRED",
          message: "Client update required."
        }),
        {
          status: 426,
          headers: {
            "content-type": "application/json",
            "x-narou-viewer-reload-required": "1"
          }
        }
      );
    const fetchMock = vi.fn().mockResolvedValue(errorResponse());
    vi.stubGlobal("fetch", fetchMock);

    await expect(deleteBookmark("bookmark-a")).rejects.toMatchObject({
      name: "ApiResponseError",
      code: "CLIENT_UPDATE_REQUIRED"
    });

    fetchMock.mockResolvedValueOnce(errorResponse());
    await expect(postReaderAiAssistantChatStream("novel-a", { message: "hello", currentEpisodeIndex: "1", position: 0 })).rejects.toMatchObject({
      name: "ApiResponseError",
      code: "CLIENT_UPDATE_REQUIRED"
    });

    fetchMock.mockResolvedValueOnce(errorResponse());
    await expect(
      postAiGenerationPlaygroundStream({
        novelId: "novel-a",
        upToEpisodeIndex: "1"
      })
    ).rejects.toMatchObject({
      name: "ApiResponseError",
      code: "CLIENT_UPDATE_REQUIRED"
    });

    vi.unstubAllGlobals();
  });

  it("does not overwrite explicit API headers", () => {
    const headers = createApiHeaders({
      "x-narou-viewer-api-contract-version": "2",
      "x-narou-viewer-client-build": "custom-build"
    });

    expect(headers.get("x-narou-viewer-api-contract-version")).toBe("2");
    expect(headers.get("x-narou-viewer-client-build")).toBe("custom-build");
  });

  it("reads AI generation Playground stream events and returns the result", async () => {
    const onProgress = vi.fn();
    const onPromptPreview = vi.fn();
    const onBatchTimings = vi.fn();
    const response = createNdjsonResponse([
      `${JSON.stringify({
        type: "status",
        stage: "generating",
        message: "生成中",
        progress: 0.5,
        step: 1,
        stepCount: 2,
        batchIndex: 2,
        batchCount: 2
      })}\n${JSON.stringify({
        type: "promptPreview",
        preview: {
          systemPrompt: "system",
          batches: []
        }
      })}\n${JSON.stringify({
        type: "batchTiming",
        batchIndex: 2,
        batchCount: 2,
        episodeIndexes: ["2"],
        chunkCount: 1,
        elapsedMs: 20,
        generatedCharacterCount: 2,
        mergedCharacterCount: 2,
        message: "2件"
      })}\n`,
      `${JSON.stringify({
        type: "batchTiming",
        batchIndex: 1,
        batchCount: 2,
        episodeIndexes: ["1"],
        chunkCount: 1,
        elapsedMs: 10,
        generatedCharacterCount: 1,
        mergedCharacterCount: 1,
        message: "1件"
      })}\n${JSON.stringify({
        type: "result",
        result: {
          novelId: "novel-a",
          novelTitle: "作品",
          upToEpisodeIndex: "2",
          processedUpToEpisodeIndex: "2",
          profileId: null,
          profileLabel: null,
          generationMode: "heuristic",
          modelId: null,
          characters: []
        }
      })}\n`
    ]);

    const result = await readAiGenerationPlaygroundStream(response, {
      onProgress,
      onPromptPreview,
      onBatchTimings
    });

    expect(result).toEqual({
      novelId: "novel-a",
      novelTitle: "作品",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: "2",
      profileId: null,
      profileLabel: null,
      generationMode: "heuristic",
      modelId: null,
      characters: []
    });
    expect(onProgress).toHaveBeenCalledWith(
      expect.objectContaining({
        stage: "generating",
        message: "生成中",
        batchIndex: 2
      })
    );
    expect(onPromptPreview).toHaveBeenCalledWith({
      systemPrompt: "system",
      batches: []
    });
    expect(onBatchTimings).toHaveBeenLastCalledWith([
      expect.objectContaining({
        batchIndex: 1
      }),
      expect.objectContaining({
        batchIndex: 2
      })
    ]);
  });

  it("reads reader assistant stream events split across chunks", async () => {
    const events: unknown[] = [];
    const response = createNdjsonResponse([
      "{\"type\":\"status\",\"message\":\"準備中\"}\n{\"type\":\"tool_call\",\"toolName\"",
      ":\"get_current_episode\",\"message\":\"本文確認\"}\n"
    ]);

    await readReaderAiAssistantStream(response, (event) => {
      events.push(event);
    });

    expect(events).toEqual([
      {
        type: "status",
        message: "準備中"
      },
      {
        type: "tool_call",
        toolName: "get_current_episode",
        message: "本文確認"
      }
    ]);
  });

  it("rejects unknown reader assistant stream events", async () => {
    const response = createNdjsonResponse([`${JSON.stringify({ type: "mystery", message: "?" })}\n`]);

    await expect(readReaderAiAssistantStream(response, () => undefined)).rejects.toThrow(
      "応答ストリームに未対応のイベントが含まれています。",
    );
  });
});
