import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';

/** Return a concrete client preference for absent or unknown wire values. */
export function timeFormatOrAuto(timeFormat: TimeFormat | null | undefined): TimeFormat {
  switch (timeFormat) {
    case TimeFormat.TIME_FORMAT_AUTO:
    case TimeFormat.TIME_FORMAT_12_HOUR:
    case TimeFormat.TIME_FORMAT_24_HOUR:
      return timeFormat;
    case TimeFormat.TIME_FORMAT_UNSPECIFIED:
    default:
      return TimeFormat.TIME_FORMAT_AUTO;
  }
}
