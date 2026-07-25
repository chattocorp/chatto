export const ROOM_NAME_MAX_LENGTH = 30;

export function normalizeRoomName(name: string): string {
  return name.trim().normalize('NFC');
}

export function roomNameCharacterCount(name: string): number {
  return [...name].length;
}

export function hasValidRoomNameCharacters(name: string): boolean {
  return /^[\p{L}\p{Nd}_-]+$/u.test(name);
}
