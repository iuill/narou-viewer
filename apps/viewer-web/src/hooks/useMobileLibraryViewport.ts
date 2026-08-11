import { useMediaQuery } from "./useMediaQuery";

const MOBILE_LIBRARY_BREAKPOINT_PX = 1000;

export function useMobileLibraryViewport() {
  return useMediaQuery(`(max-width: ${MOBILE_LIBRARY_BREAKPOINT_PX}px)`);
}
