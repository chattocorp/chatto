import { Codecs, globalSlot } from './slot';

export const ROOM_INFO_DEFAULT_WIDTH = 256;
export const ROOM_INFO_MIN_WIDTH = 200;
export const ROOM_INFO_MAX_WIDTH = 480;

const slot = globalSlot(
  'roomInfoWidth',
  ROOM_INFO_DEFAULT_WIDTH,
  Codecs.number({ min: ROOM_INFO_MIN_WIDTH, max: ROOM_INFO_MAX_WIDTH })
);

export function getRoomInfoWidth(): number {
  return slot.get();
}

export function setRoomInfoWidth(width: number): void {
  const clamped = Math.min(ROOM_INFO_MAX_WIDTH, Math.max(ROOM_INFO_MIN_WIDTH, width));
  slot.set(clamped);
}
