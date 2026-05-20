import {
  getRoomSidebarWidth,
  setRoomSidebarWidth,
  ROOM_SIDEBAR_DEFAULT_WIDTH,
  ROOM_SIDEBAR_MAX_WIDTH,
  ROOM_SIDEBAR_MIN_WIDTH
} from '$lib/storage/roomSidebarWidth';

class RoomSidebarWidthState {
  #width = $state(getRoomSidebarWidth());

  get value(): number {
    return this.#width;
  }

  set(width: number): void {
    const clamped = Math.min(ROOM_SIDEBAR_MAX_WIDTH, Math.max(ROOM_SIDEBAR_MIN_WIDTH, width));
    this.#width = clamped;
    setRoomSidebarWidth(clamped);
  }

  reset(): void {
    this.set(ROOM_SIDEBAR_DEFAULT_WIDTH);
  }
}

export const roomSidebarWidth = new RoomSidebarWidthState();
