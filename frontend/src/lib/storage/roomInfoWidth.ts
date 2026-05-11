const STORAGE_KEY = 'chatto:roomInfoWidth';

export const ROOM_INFO_DEFAULT_WIDTH = 256;
export const ROOM_INFO_MIN_WIDTH = 200;
export const ROOM_INFO_MAX_WIDTH = 480;

export function getRoomInfoWidth(): number {
  if (typeof localStorage === 'undefined') {
    return ROOM_INFO_DEFAULT_WIDTH;
  }
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const value = parseFloat(stored);
      if (
        !isNaN(value) &&
        value >= ROOM_INFO_MIN_WIDTH &&
        value <= ROOM_INFO_MAX_WIDTH
      ) {
        return value;
      }
    }
  } catch {
    // Ignore storage errors
  }
  return ROOM_INFO_DEFAULT_WIDTH;
}

export function setRoomInfoWidth(width: number): void {
  const clamped = Math.min(ROOM_INFO_MAX_WIDTH, Math.max(ROOM_INFO_MIN_WIDTH, width));
  if (typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, String(clamped));
  } catch {
    // Ignore storage errors
  }
}
