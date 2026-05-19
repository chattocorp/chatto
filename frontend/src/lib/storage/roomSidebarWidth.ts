import { Codecs, globalSlot } from './slot';

export const ROOM_SIDEBAR_DEFAULT_WIDTH = 256;
export const ROOM_SIDEBAR_MIN_WIDTH = 200;
export const ROOM_SIDEBAR_MAX_WIDTH = 480;

const slot = globalSlot(
  'roomSidebarWidth',
  ROOM_SIDEBAR_DEFAULT_WIDTH,
  Codecs.number({ min: ROOM_SIDEBAR_MIN_WIDTH, max: ROOM_SIDEBAR_MAX_WIDTH })
);

export function getRoomSidebarWidth(): number {
  return slot.get();
}

export function setRoomSidebarWidth(width: number): void {
  const clamped = Math.min(ROOM_SIDEBAR_MAX_WIDTH, Math.max(ROOM_SIDEBAR_MIN_WIDTH, width));
  slot.set(clamped);
}
