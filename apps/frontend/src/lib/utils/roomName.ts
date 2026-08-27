export const ROOM_NAME_MAX_LENGTH = 30;

export function normalizeRoomName(name: string): string {
  return name.trim().normalize('NFC');
}

export function roomNameCharacterCount(name: string): number {
  return [...name].length;
}

export type RoomNameValidationError = 'empty' | 'too_long' | 'invalid';

/**
 * Validates the user-visible room name using the same rules as the server.
 * Format characters remain available for scripts and emoji sequences, but do
 * not count as visible content by themselves.
 */
export function roomNameValidationError(name: string): RoomNameValidationError | undefined {
  const normalized = normalizeRoomName(name);
  if (!normalized) return 'empty';
  if (roomNameCharacterCount(normalized) > ROOM_NAME_MAX_LENGTH) return 'too_long';
  if (/[\p{Cc}\p{Zl}\p{Zp}]/u.test(normalized)) return 'invalid';
  if (!/[^\p{White_Space}\p{Cf}\p{Cc}]/u.test(normalized)) return 'empty';
  return undefined;
}
