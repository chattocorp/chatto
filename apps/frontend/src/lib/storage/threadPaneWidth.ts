import { paneWidthSlot } from './paneWidth';

export const THREAD_PANE_DEFAULT_WIDTH = 420;
export const THREAD_PANE_MIN_WIDTH = 280;
export const THREAD_PANE_MAX_WIDTH = 720;

export const threadPaneWidthSlot = paneWidthSlot('threadPaneWidth', {
  defaultValue: THREAD_PANE_DEFAULT_WIDTH,
  minWidth: THREAD_PANE_MIN_WIDTH,
  maxWidth: THREAD_PANE_MAX_WIDTH
});
