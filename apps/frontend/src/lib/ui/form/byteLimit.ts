const textEncoder = new TextEncoder();
const graphemes = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

/**
 * Returns `value` cut at the end, in whole characters, so that its UTF-8
 * encoding fits `maxBytes`. Server limits count bytes, and `maxlength` counts
 * UTF-16 code units, so `maxlength` alone accepts text that the server rejects.
 */
export function fitToBytes(value: string, maxBytes: number): string {
  if (textEncoder.encode(value).byteLength <= maxBytes) return value;
  let result = '';
  let used = 0;
  for (const { segment } of graphemes.segment(value)) {
    used += textEncoder.encode(segment).byteLength;
    if (used > maxBytes) break;
    result += segment;
  }
  return result;
}
