import { Codecs, globalSlot } from './slot';

export const SECONDARY_SIDEBAR_DEFAULT_WIDTH = 256;
export const SECONDARY_SIDEBAR_MIN_WIDTH = 200;
export const SECONDARY_SIDEBAR_MAX_WIDTH = 480;

const slot = globalSlot(
  'secondarySidebarWidth',
  SECONDARY_SIDEBAR_DEFAULT_WIDTH,
  Codecs.number({ min: SECONDARY_SIDEBAR_MIN_WIDTH, max: SECONDARY_SIDEBAR_MAX_WIDTH })
);

export function getSecondarySidebarWidth(): number {
  return slot.get();
}

export function setSecondarySidebarWidth(width: number): void {
  const clamped = Math.min(
    SECONDARY_SIDEBAR_MAX_WIDTH,
    Math.max(SECONDARY_SIDEBAR_MIN_WIDTH, width)
  );
  slot.set(clamped);
}
