export type ReaderFullscreenToggleAction =
  | "noop"
  | "exit-native"
  | "disable-pseudo"
  | "enable-pseudo"
  | "request-native";

type FullscreenCapableDocument = Document & {
  webkitExitFullscreen?: () => Promise<void> | void;
  webkitFullscreenElement?: Element | null;
};

type FullscreenCapableElement = HTMLElement & {
  webkitRequestFullscreen?: () => Promise<void> | void;
};

function getDefaultDocument(): Document | undefined {
  return typeof document === "undefined" ? undefined : document;
}

export function getFullscreenElement(doc: Document | undefined = getDefaultDocument()): Element | null {
  if (!doc) {
    return null;
  }

  const fullscreenDocument = doc as FullscreenCapableDocument;

  return fullscreenDocument.fullscreenElement ?? fullscreenDocument.webkitFullscreenElement ?? null;
}

export function supportsElementFullscreen(element: HTMLElement): boolean {
  const fullscreenElement = element as FullscreenCapableElement;

  return (
    typeof fullscreenElement.requestFullscreen === "function" ||
    typeof fullscreenElement.webkitRequestFullscreen === "function"
  );
}

export async function requestElementFullscreen(element: HTMLElement): Promise<void> {
  const fullscreenElement = element as FullscreenCapableElement;

  if (typeof fullscreenElement.requestFullscreen === "function") {
    await fullscreenElement.requestFullscreen();
    return;
  }

  if (typeof fullscreenElement.webkitRequestFullscreen === "function") {
    await fullscreenElement.webkitRequestFullscreen();
    return;
  }

  throw new Error("Fullscreen API is not available.");
}

export async function exitDocumentFullscreen(doc: Document | undefined = getDefaultDocument()): Promise<void> {
  if (!doc) {
    return;
  }

  const fullscreenDocument = doc as FullscreenCapableDocument;

  if (typeof doc.exitFullscreen === "function") {
    await doc.exitFullscreen();
    return;
  }

  if (typeof fullscreenDocument.webkitExitFullscreen === "function") {
    await fullscreenDocument.webkitExitFullscreen();
  }
}

export function resolveReaderFullscreenToggleAction(input: {
  hasReaderShell: boolean;
  isNativeFullscreen: boolean;
  isPseudoFullscreen: boolean;
  supportsNativeFullscreen: boolean;
}): ReaderFullscreenToggleAction {
  if (!input.hasReaderShell) {
    return "noop";
  }

  if (input.isNativeFullscreen) {
    return "exit-native";
  }

  if (input.isPseudoFullscreen) {
    return "disable-pseudo";
  }

  if (!input.supportsNativeFullscreen) {
    return "enable-pseudo";
  }

  return "request-native";
}

export function shouldExitReaderFullscreenOnReturn(input: {
  hasReaderShell: boolean;
  isNativeFullscreen: boolean;
}): boolean {
  return input.hasReaderShell && input.isNativeFullscreen;
}
