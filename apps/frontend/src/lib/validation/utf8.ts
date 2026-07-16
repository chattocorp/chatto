const encoder = new TextEncoder();

/** Return the number of bytes used by a string when encoded as UTF-8. */
export function utf8ByteLength(value: string): number {
  return encoder.encode(value).byteLength;
}
