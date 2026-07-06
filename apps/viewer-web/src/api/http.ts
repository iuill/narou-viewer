import { viewerBuildInfo } from "../buildInfo";
import { createApiContractHeaders } from "./contract";
import { createApiResponseError, type ApiResponseError } from "./errors";

function resolveApiClientBuild(): string {
  const version = viewerBuildInfo.version.trim() || "0.0.0";
  return viewerBuildInfo.gitShortHash ? `${version}+${viewerBuildInfo.gitShortHash}` : version;
}

export function createApiHeaders(headers?: HeadersInit): Headers {
  const nextHeaders = new Headers(headers);
  const contractHeaders = createApiContractHeaders(resolveApiClientBuild());
  for (const [key, value] of Object.entries(contractHeaders)) {
    if (!nextHeaders.has(key)) {
      nextHeaders.set(key, value);
    }
  }
  return nextHeaders;
}

function createApiRequestInit(init: RequestInit = {}): RequestInit {
  return {
    ...init,
    headers: createApiHeaders(init.headers)
  };
}

export function apiFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  return fetch(input, createApiRequestInit(init));
}

export function parseApiResponseBody(bodyText: string, contentType: string, fallbackMessage: string): unknown {
  const hasBody = bodyText.trim().length > 0;
  const shouldParseJson = hasBody && contentType.toLowerCase().includes("json");
  if (!shouldParseJson) {
    return undefined;
  }
  try {
    return JSON.parse(bodyText) as unknown;
  } catch {
    throw new Error(fallbackMessage);
  }
}

export async function readApiResponse<T>(response: Response, fallbackMessage: string): Promise<T> {
  const bodyText = await response.text();
  const contentType = response.headers.get("content-type") ?? "";
  const hasBody = bodyText.trim().length > 0;
  const shouldParseJson = hasBody && contentType.toLowerCase().includes("json");
  let payload: unknown;

  if (shouldParseJson) {
    try {
      payload = JSON.parse(bodyText) as unknown;
    } catch {
      if (response.ok) {
        throw new Error(fallbackMessage);
      }
    }
  }

  if (!response.ok) {
    throw createApiResponseError(response, payload, shouldParseJson ? "" : bodyText, fallbackMessage);
  }

  if (!hasBody) {
    return undefined as T;
  }
  if (payload !== undefined) {
    return payload as T;
  }

  try {
    return JSON.parse(bodyText) as T;
  } catch {
    throw new Error(fallbackMessage);
  }
}

export async function requestJson<T>(
  input: RequestInfo | URL,
  init: RequestInit | undefined,
  fallbackMessage: string
): Promise<T> {
  const response = await apiFetch(input, init);
  return readApiResponse<T>(response, fallbackMessage);
}

export async function mutateJson<TResponse, TRequest>(
  input: RequestInfo | URL,
  body: TRequest,
  fallbackMessage: string
): Promise<TResponse> {
  return requestJson<TResponse>(
    input,
    {
      method: "POST",
      headers: {
        "content-type": "application/json"
      },
      body: JSON.stringify(body)
    },
    fallbackMessage
  );
}

export async function createApiResponseErrorFromResponse(
  response: Response,
  fallbackMessage: string
): Promise<ApiResponseError> {
  const bodyText = await response.text();
  const contentType = response.headers.get("content-type") ?? "";
  let payload: unknown;
  try {
    payload = parseApiResponseBody(bodyText, contentType, fallbackMessage);
  } catch {
    payload = undefined;
  }
  return createApiResponseError(response, payload, contentType.toLowerCase().includes("json") ? "" : bodyText, fallbackMessage);
}
