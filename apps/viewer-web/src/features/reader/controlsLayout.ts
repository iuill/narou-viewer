const READER_CONTROL_BUTTON_SIZE_PX = 42;
const READER_CONTROL_DESKTOP_GAP_PX = 8;
const READER_CONTROL_MOBILE_GAP_PX = 6;
const READER_CONTROL_DESKTOP_HORIZONTAL_PADDING_PX = 16;
const READER_CONTROL_MOBILE_HORIZONTAL_PADDING_PX = 20;
const READER_CONTROL_MIN_VISIBLE_ACTION_SLOTS = 4;

export function getReaderControlCapacity(
  viewportWidth: number,
  isCompactViewport: boolean,
  trailingReservedWidth: number = 0,
  buttonSizePx: number = READER_CONTROL_BUTTON_SIZE_PX
): number {
  const gap = isCompactViewport ? READER_CONTROL_MOBILE_GAP_PX : READER_CONTROL_DESKTOP_GAP_PX;
  const horizontalPadding = isCompactViewport
    ? READER_CONTROL_MOBILE_HORIZONTAL_PADDING_PX
    : READER_CONTROL_DESKTOP_HORIZONTAL_PADDING_PX;
  const usableWidth = Math.max(viewportWidth - horizontalPadding - trailingReservedWidth, buttonSizePx);

  return Math.max(
    READER_CONTROL_MIN_VISIBLE_ACTION_SLOTS,
    Math.floor((usableWidth + gap) / (buttonSizePx + gap))
  );
}
