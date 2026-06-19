import { TimeFormat as WireTimeFormat } from '$lib/pb/chatto/core/v1/user_preferences_pb';

export enum TimeFormat {
  Auto = 'AUTO',
  TwelveHour = 'TWELVE_HOUR',
  TwentyFourHour = 'TWENTY_FOUR_HOUR'
}

export function timeFormatFromWire(format: WireTimeFormat | undefined | null): TimeFormat {
  switch (format) {
    case WireTimeFormat.TIME_FORMAT_12H:
      return TimeFormat.TwelveHour;
    case WireTimeFormat.TIME_FORMAT_24H:
      return TimeFormat.TwentyFourHour;
    case WireTimeFormat.TIME_FORMAT_UNSPECIFIED:
    default:
      return TimeFormat.Auto;
  }
}

export function timeFormatToWire(format: TimeFormat): WireTimeFormat {
  switch (format) {
    case TimeFormat.TwelveHour:
      return WireTimeFormat.TIME_FORMAT_12H;
    case TimeFormat.TwentyFourHour:
      return WireTimeFormat.TIME_FORMAT_24H;
    case TimeFormat.Auto:
    default:
      return WireTimeFormat.TIME_FORMAT_UNSPECIFIED;
  }
}

export function normalizeTimeFormat(
  value: TimeFormat | string | WireTimeFormat | null | undefined
): TimeFormat {
  if (typeof value === 'number') {
    return timeFormatFromWire(value);
  }
  switch (value) {
    case TimeFormat.TwelveHour:
      return TimeFormat.TwelveHour;
    case TimeFormat.TwentyFourHour:
      return TimeFormat.TwentyFourHour;
    case TimeFormat.Auto:
    default:
      return TimeFormat.Auto;
  }
}
