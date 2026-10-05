/*
 * Timestamp tokens in message bodies. A token such as `<t:1745764200:F>`
 * marks an instant that each reader sees in their own time zone. Message
 * bodies keep the literal token; hosts render it.
 */

/** Largest Unix second that a JavaScript `Date` can represent. */
const MAX_UNIX_SECONDS = Math.floor(8.64e15 / 1000);

const TOKEN_SOURCE = String.raw`<t:(\d{1,12}):F>`;
const EXACT_TOKEN = new RegExp(`^${TOKEN_SOURCE}$`);

/** A parsed timestamp token. `F` (full date and time) is the only format. */
export type MessageTimestampToken = {
  epochSeconds: number;
  format: 'F';
};

function isSupportedEpochSeconds(epochSeconds: number): boolean {
  return (
    Number.isSafeInteger(epochSeconds) && epochSeconds >= 0 && epochSeconds <= MAX_UNIX_SECONDS
  );
}

/**
 * Returns a new global pattern that finds token candidates in text. Each call
 * returns a new `RegExp`, so callers do not share `lastIndex` state. Pass each
 * match to `parseMessageTimestampToken` to validate it.
 */
export function messageTimestampTokenPattern(): RegExp {
  return new RegExp(TOKEN_SOURCE, 'g');
}

/** Parses one complete token, or returns null for other text and unsupported values. */
export function parseMessageTimestampToken(value: string): MessageTimestampToken | null {
  const match = value.match(EXACT_TOKEN);
  if (!match) return null;
  const epochSeconds = Number(match[1]);
  if (!isSupportedEpochSeconds(epochSeconds)) return null;
  return { epochSeconds, format: 'F' };
}

/**
 * Creates the token for a Unix time in seconds.
 * @throws RangeError when the value is not a whole number of seconds that `Date` supports.
 */
export function createMessageTimestampToken(epochSeconds: number): string {
  if (!isSupportedEpochSeconds(epochSeconds)) {
    throw new RangeError('Timestamp is outside the supported Unix seconds range');
  }
  return `<t:${epochSeconds}:F>`;
}
