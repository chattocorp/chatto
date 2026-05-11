import {
  getRoomInfoWidth,
  setRoomInfoWidth,
  ROOM_INFO_DEFAULT_WIDTH,
  ROOM_INFO_MAX_WIDTH,
  ROOM_INFO_MIN_WIDTH
} from '$lib/storage/roomInfoWidth';

class RoomInfoWidthState {
  #width = $state(getRoomInfoWidth());

  get value(): number {
    return this.#width;
  }

  set(width: number): void {
    const clamped = Math.min(ROOM_INFO_MAX_WIDTH, Math.max(ROOM_INFO_MIN_WIDTH, width));
    this.#width = clamped;
    setRoomInfoWidth(clamped);
  }

  reset(): void {
    this.set(ROOM_INFO_DEFAULT_WIDTH);
  }
}

export const roomInfoWidth = new RoomInfoWidthState();
