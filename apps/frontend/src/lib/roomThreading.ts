import { RoomThreadingMode } from '@chatto/api-types/api/v1/common_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';

export { RoomThreadingMode };

export function normalizeRoomThreadingMode(
  kind: RoomKind,
  mode: RoomThreadingMode | undefined
): RoomThreadingMode {
  if (kind === RoomKind.DM) return RoomThreadingMode.UNSPECIFIED;
  if (mode === undefined || mode === RoomThreadingMode.UNSPECIFIED) {
    return RoomThreadingMode.ENABLED;
  }
  switch (mode) {
    case RoomThreadingMode.REQUIRED:
    case RoomThreadingMode.ENCOURAGED:
    case RoomThreadingMode.ENABLED:
    case RoomThreadingMode.DISABLED:
      return mode;
    default:
      return RoomThreadingMode.DISABLED;
  }
}
