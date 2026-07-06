import { expect } from "vitest";

export type ContractResponse<T = unknown> = {
  status: number;
  contentType: string;
  etag: string | null;
  deprecation: string | null;
  link: string | null;
  bodyText: string;
  json: T;
};

export type ContractRawResponse = Omit<ContractResponse, "json">;

type RequestOptions = {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
};

const rawApiBaseUrl = process.env.API_BASE_URL?.trim();
const API_CONTRACT_VERSION = "1";

if (!rawApiBaseUrl) {
  throw new Error(
    "API_BASE_URL is required for api-contract tests. Example: API_BASE_URL=http://127.0.0.1:18080 bun run test:api-contract",
  );
}

export const API_BASE_URL = normalizeBaseUrl(rawApiBaseUrl);
export const MUTATING_CONTRACT_TESTS_ENABLED =
  process.env.API_CONTRACT_MUTATING === "1";
export const DESTRUCTIVE_FETCHER_CONTRACT_TESTS_ENABLED =
  process.env.API_CONTRACT_DESTRUCTIVE_FETCHER === "1";
export const REQUIRE_CONTRACT_FIXTURE =
  process.env.API_CONTRACT_REQUIRE_FIXTURE === "1";

const STRICT_UTC_ISO_TIMESTAMP_PATTERN =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/;

function normalizeBaseUrl(value: string): string {
  const trimmed = value.trim();
  return trimmed.endsWith("/") ? trimmed.slice(0, -1) : trimmed;
}

export function contractUrl(pathname: string): string {
  return `${API_BASE_URL}${pathname.startsWith("/") ? pathname : `/${pathname}`}`;
}

export async function requestRaw(
  pathname: string,
  options: RequestOptions = {},
): Promise<ContractRawResponse> {
  const headers = new Headers(options.headers);
  if (!headers.has("x-narou-viewer-api-contract-version")) {
    headers.set("x-narou-viewer-api-contract-version", API_CONTRACT_VERSION);
  }
  if (!headers.has("x-narou-viewer-client-build")) {
    headers.set("x-narou-viewer-client-build", "api-contract-local");
  }
  if (options.body !== undefined && !headers.has("content-type")) {
    headers.set("content-type", "application/json");
  }

  const response = await fetch(contractUrl(pathname), {
    method: options.method,
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  const bodyText = await response.text();
  const contentType = response.headers.get("content-type") ?? "";

  return {
    status: response.status,
    contentType,
    etag: response.headers.get("etag"),
    deprecation: response.headers.get("deprecation"),
    link: response.headers.get("link"),
    bodyText,
  };
}

export async function requestJson<T = unknown>(
  pathname: string,
  options: RequestOptions = {},
): Promise<ContractResponse<T>> {
  const rawResponse = await requestRaw(pathname, options);
  let json: T;

  try {
    json = JSON.parse(rawResponse.bodyText) as T;
  } catch (error) {
    throw new Error(
      `Expected JSON response from ${pathname}, got ${rawResponse.contentType}: ${rawResponse.bodyText}`,
      {
        cause: error,
      },
    );
  }

  return {
    ...rawResponse,
    json,
  };
}

export function parseNdjson(bodyText: string): unknown[] {
  return bodyText
    .trim()
    .split("\n")
    .filter((line) => line.trim().length > 0)
    .map((line) => JSON.parse(line) as unknown);
}

export function expectJsonResponse(
  response: Pick<ContractResponse, "status" | "contentType">,
  status = 200,
): void {
  expect(response.status).toBe(status);
  expect(response.contentType).toContain("application/json");
}

export function expectErrorShape(payload: unknown): void {
  expect(payload).toEqual(
    expect.objectContaining({
      error: expect.any(String),
      code: expect.any(String),
      message: expect.any(String),
    }),
  );
}

export function expectNonEmptyFixtureArray(
  items: unknown[],
  label: string,
): void {
  if (!REQUIRE_CONTRACT_FIXTURE) {
    return;
  }

  expect(
    items.length,
    `${label} fixture must not be empty when API_CONTRACT_REQUIRE_FIXTURE=1`,
  ).toBeGreaterThan(0);
}

export function expectIsoStringOrNull(value: unknown): void {
  if (value === null) {
    return;
  }
  expect(typeof value).toBe("string");
  expect(value).toMatch(STRICT_UTC_ISO_TIMESTAMP_PATTERN);
  expect(Number.isNaN(Date.parse(value as string))).toBe(false);
}
