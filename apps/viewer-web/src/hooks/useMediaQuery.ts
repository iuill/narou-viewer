import { useEffect, useState } from "react";

function getWindow(): Window | null {
  return typeof window === "undefined" ? null : window;
}

function getMediaQueryList(query: string): MediaQueryList | null {
  const currentWindow = getWindow();
  return currentWindow && typeof currentWindow.matchMedia === "function" ? currentWindow.matchMedia(query) : null;
}

export function useMediaQuery(query: string): boolean {
  const [matches, setMatches] = useState(() => getMediaQueryList(query)?.matches ?? false);

  useEffect(() => {
    const mediaQueryList = getMediaQueryList(query);
    if (!mediaQueryList) {
      setMatches(false);
      return;
    }

    const updateMatches = () => setMatches(mediaQueryList.matches);
    updateMatches();

    if (typeof mediaQueryList.addEventListener === "function") {
      mediaQueryList.addEventListener("change", updateMatches);
      return () => mediaQueryList.removeEventListener("change", updateMatches);
    }

    mediaQueryList.addListener(updateMatches);
    return () => mediaQueryList.removeListener(updateMatches);
  }, [query]);

  return matches;
}

export function useTouchDevice(): boolean {
  const hasCoarsePointer = useMediaQuery("(pointer: coarse)");
  const [hasTouchPoints, setHasTouchPoints] = useState(() => (getWindow()?.navigator.maxTouchPoints ?? 0) > 0);

  useEffect(() => {
    setHasTouchPoints((getWindow()?.navigator.maxTouchPoints ?? 0) > 0);
  }, []);

  return hasCoarsePointer || hasTouchPoints;
}
