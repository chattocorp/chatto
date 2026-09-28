import { timestampDate, type Timestamp } from '@bufbuild/protobuf/wkt';

/** The date of an optional protobuf timestamp; `undefined` when the field is unset. */
export function timestampToDate(timestamp: Timestamp | undefined): Date | undefined {
  return timestamp ? timestampDate(timestamp) : undefined;
}

/** ISO-8601 text of an optional protobuf timestamp; `null` when the field is unset. */
export function timestampToISO(timestamp: Timestamp | undefined): string | null {
  return timestampToDate(timestamp)?.toISOString() ?? null;
}
