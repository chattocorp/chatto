import { Codecs, globalSlot } from './slot';

export const SERVER_SIDEBAR_DEFAULT_WIDTH = 256;
export const SERVER_SIDEBAR_MIN_WIDTH = 200;
export const SERVER_SIDEBAR_MAX_WIDTH = 480;

const slot = globalSlot(
  'serverSidebarWidth',
  SERVER_SIDEBAR_DEFAULT_WIDTH,
  Codecs.number({ min: SERVER_SIDEBAR_MIN_WIDTH, max: SERVER_SIDEBAR_MAX_WIDTH })
);

export function getServerSidebarWidth(): number {
  return slot.get();
}

export function setServerSidebarWidth(width: number): void {
  const clamped = Math.min(
    SERVER_SIDEBAR_MAX_WIDTH,
    Math.max(SERVER_SIDEBAR_MIN_WIDTH, width)
  );
  slot.set(clamped);
}
