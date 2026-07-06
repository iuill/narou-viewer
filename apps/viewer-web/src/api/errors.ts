import {
  API_CLIENT_UPDATE_REQUIRED_EVENT,
  API_RELOAD_REQUIRED_HEADER,
  type ApiClientUpdateRequiredEventDetail
} from "./contract";

type ApiErrorPayload = {
  error?: unknown;
  code?: unknown;
  message?: unknown;
  details?: unknown;
  requestId?: unknown;
};

export class ApiResponseError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string | null,
    readonly details: unknown,
    readonly requestId: string | null,
    readonly reloadRequired: boolean
  ) {
    super(message);
    this.name = "ApiResponseError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function extractApiErrorPayload(payload: unknown): ApiErrorPayload | null {
  if (!isRecord(payload)) {
    return null;
  }
  return payload;
}

function extractApiErrorMessage(value: unknown): string | null {
  if (typeof value === "string") {
    const trimmed = value.trim();
    return trimmed.length > 0 ? trimmed : null;
  }
  if (!isRecord(value)) {
    return null;
  }
  return extractApiErrorMessage(value.message) ?? extractApiErrorMessage(value.error);
}

function extractMinApiContractVersion(details: unknown): string | null {
  if (!isRecord(details)) {
    return null;
  }
  const value = details.minApiContractVersion;
  return typeof value === "string" && value.trim().length > 0 ? value.trim() : null;
}

function notifyClientUpdateRequired(error: ApiResponseError): void {
  if (typeof window === "undefined" || (!error.reloadRequired && error.code !== "CLIENT_UPDATE_REQUIRED")) {
    return;
  }
  const detail: ApiClientUpdateRequiredEventDetail = {
    message: error.message,
    code: error.code,
    status: error.status,
    minApiContractVersion: extractMinApiContractVersion(error.details)
  };
  window.dispatchEvent(new CustomEvent(API_CLIENT_UPDATE_REQUIRED_EVENT, { detail }));
}

export function createApiResponseError(
  response: Response,
  payload: unknown,
  bodyText: string,
  fallbackMessage: string
): ApiResponseError {
  const apiError = extractApiErrorPayload(payload);
  const message =
    extractApiErrorMessage(apiError?.error) ??
    extractApiErrorMessage(apiError?.message) ??
    (bodyText.trim().length > 0 ? bodyText.trim() : fallbackMessage);
  const code = typeof apiError?.code === "string" && apiError.code.trim().length > 0 ? apiError.code.trim() : null;
  const requestId =
    typeof apiError?.requestId === "string" && apiError.requestId.trim().length > 0 ? apiError.requestId.trim() : null;
  const reloadRequired = response.headers.get(API_RELOAD_REQUIRED_HEADER) === "1";
  const error = new ApiResponseError(message, response.status, code, apiError?.details, requestId, reloadRequired);
  notifyClientUpdateRequired(error);
  return error;
}

export function formatApiResponseError(
  error: ApiResponseError,
  formatMessage: (message: string) => string
): ApiResponseError {
  return new ApiResponseError(
    formatMessage(error.message),
    error.status,
    error.code,
    error.details,
    error.requestId,
    error.reloadRequired
  );
}
