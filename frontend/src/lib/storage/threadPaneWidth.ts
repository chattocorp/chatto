import { Codecs, globalSlot } from './slot';

export const THREAD_PANE_MIN_WIDTH = 25;
export const THREAD_PANE_MAX_WIDTH = 75;
const DEFAULT_WIDTH = 50; // percentage

const slot = globalSlot(
  'threadPaneWidth',
  DEFAULT_WIDTH,
  Codecs.number({ min: THREAD_PANE_MIN_WIDTH, max: THREAD_PANE_MAX_WIDTH })
);

export function getThreadPaneWidth(): number {
  return slot.get();
}

export function setThreadPaneWidth(width: number): void {
  const clamped = Math.min(THREAD_PANE_MAX_WIDTH, Math.max(THREAD_PANE_MIN_WIDTH, width));
  slot.set(clamped);
}
