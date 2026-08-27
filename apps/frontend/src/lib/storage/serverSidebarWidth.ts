import { paneWidthSlot } from './paneWidth';

export const SERVER_SIDEBAR_DEFAULT_WIDTH = 256;
export const SERVER_SIDEBAR_MIN_WIDTH = 200;
export const SERVER_SIDEBAR_MAX_WIDTH = 480;

export const serverSidebarWidthSlot = paneWidthSlot('serverSidebarWidth', {
  defaultValue: SERVER_SIDEBAR_DEFAULT_WIDTH,
  minWidth: SERVER_SIDEBAR_MIN_WIDTH,
  maxWidth: SERVER_SIDEBAR_MAX_WIDTH
});
