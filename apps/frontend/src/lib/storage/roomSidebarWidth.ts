import { paneWidthSlot } from './paneWidth';

export const ROOM_SIDEBAR_DEFAULT_WIDTH = 256;
export const ROOM_SIDEBAR_MIN_WIDTH = 200;
export const ROOM_SIDEBAR_MAX_WIDTH = 480;

export const roomSidebarWidthSlot = paneWidthSlot('roomSidebarWidth', {
  defaultValue: ROOM_SIDEBAR_DEFAULT_WIDTH,
  minWidth: ROOM_SIDEBAR_MIN_WIDTH,
  maxWidth: ROOM_SIDEBAR_MAX_WIDTH
});
