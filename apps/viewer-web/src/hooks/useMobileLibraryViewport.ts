import { useMediaQuery, useTouchDevice } from "./useMediaQuery";

const MOBILE_LIBRARY_BREAKPOINT_PX = 800;
const TOUCH_LIBRARY_BREAKPOINT_PX = 1100;

export function useMobileLibraryViewport() {
  const isCompactViewport = useMediaQuery(`(max-width: ${MOBILE_LIBRARY_BREAKPOINT_PX}px)`);
  const isTouchDevice = useTouchDevice();
  const isTouchViewport = useMediaQuery(`(max-width: ${TOUCH_LIBRARY_BREAKPOINT_PX}px)`);

  return isCompactViewport || (isTouchDevice && isTouchViewport);
}
