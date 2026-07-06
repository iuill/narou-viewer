export const API_CONTRACT_VERSION = "1";
export const API_CONTRACT_VERSION_HEADER = "x-narou-viewer-api-contract-version";
export const API_CONTRACT_MIN_VERSION_HEADER = "x-narou-viewer-min-api-contract-version";
export const API_CLIENT_BUILD_HEADER = "x-narou-viewer-client-build";
export const API_RELOAD_REQUIRED_HEADER = "x-narou-viewer-reload-required";
export const API_CLIENT_UPDATE_REQUIRED_EVENT = "narou-viewer:client-update-required";

export type ApiClientUpdateRequiredEventDetail = {
  message: string;
  code: string | null;
  status: number;
  minApiContractVersion: string | null;
};

export function createApiContractHeaders(clientBuild: string): Record<string, string> {
  return {
    [API_CONTRACT_VERSION_HEADER]: API_CONTRACT_VERSION,
    [API_CLIENT_BUILD_HEADER]: clientBuild
  };
}
