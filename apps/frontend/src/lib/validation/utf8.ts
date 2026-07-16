const encoder = new TextEncoder();

/** Return the number of bytes used by a string when encoded as UTF-8. */
export function utf8ByteLength(value: string): number {
  return encoder.encode(value).byteLength;
}

/**
 * Keep the longest prefix of a string whose UTF-8 encoding fits within the
 * supplied byte limit. Iterating the string preserves complete code points.
 */
export function truncateUtf8(value: string, maxBytes: number): string {
  if (utf8ByteLength(value) <= maxBytes) return value;

  let result = '';
  let bytes = 0;
  for (const character of value) {
    const characterBytes = utf8ByteLength(character);
    if (bytes + characterBytes > maxBytes) break;
    result += character;
    bytes += characterBytes;
  }
  return result;
}
