export type E2EControlStore = {
  disableReaderStateSave?: boolean;
};

export const E2E_CONTROL_WINDOW_KEY = "__NAROU_VIEWER_E2E__";

function getDefaultWindow(): (Window & Record<string, unknown>) | undefined {
  return typeof window === "undefined" ? undefined : (window as unknown as Window & Record<string, unknown>);
}

export function getE2EControlStore(currentWindow: (Window & Record<string, unknown>) | undefined = getDefaultWindow()): E2EControlStore | null {
  if (!currentWindow) {
    return null;
  }

  const store = currentWindow[E2E_CONTROL_WINDOW_KEY];

  if (!store || typeof store !== "object") {
    return null;
  }

  return store as E2EControlStore;
}

export function isReaderStateSaveDisabled(
  currentWindow: (Window & Record<string, unknown>) | undefined = getDefaultWindow()
): boolean {
  return getE2EControlStore(currentWindow)?.disableReaderStateSave === true;
}
