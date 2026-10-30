/** Shared room-kind icons for names, navigation, links, and composer suggestions. */
import { RoomKind } from '@chatto/client/api/roomDirectory';

/**
 * Return the room's icon classes and fallback text. Ordinary channels use the
 * text hash; universal channels use a globe, and group DMs use speech bubbles.
 * Callers own sizing, accessible labels, and membership/action indicators.
 */
export function roomIcon(room: { type?: RoomKind; isUniversal?: boolean }): {
  className: string;
  text: string;
} {
  if (room.type === RoomKind.DM) return { className: 'iconify icon-[uil--comments]', text: '' };
  if (room.isUniversal) return { className: 'iconify icon-[uil--globe]', text: '' };
  return { className: '', text: '#' };
}
