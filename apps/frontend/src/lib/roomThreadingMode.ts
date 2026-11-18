import { m } from '$lib/i18n/messages';
import { RoomThreadingMode } from '@chatto/client/util/roomThreading';

/**
 * Returns the localized name of a room threading mode.
 *
 * An unknown mode from a newer server gets the "Disabled" label, which matches
 * how `normalizeRoomThreadingMode` treats it.
 */
export function threadingModeLabel(mode: RoomThreadingMode): string {
  switch (mode) {
    case RoomThreadingMode.REQUIRED:
      return m('room.management.threading_mode_required');
    case RoomThreadingMode.ENCOURAGED:
      return m('room.management.threading_mode_encouraged');
    case RoomThreadingMode.ENABLED:
      return m('room.management.threading_mode_enabled');
    default:
      return m('room.management.threading_mode_disabled');
  }
}
