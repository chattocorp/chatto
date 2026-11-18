import { m } from '$lib/i18n/messages';
import { RoomThreadingMode } from '@chatto/client/util/roomThreading';

/**
 * Returns the localized name of a room threading mode.
 *
 * A mode that this client does not know, including `UNSPECIFIED`, gets the
 * "Disabled" label. Normalize room data with `normalizeRoomThreadingMode` first.
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
