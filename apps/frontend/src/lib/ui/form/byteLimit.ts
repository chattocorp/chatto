const textEncoder = new TextEncoder();
const graphemes = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

type TextField = HTMLInputElement | HTMLTextAreaElement;

function byteLength(value: string): number {
  return textEncoder.encode(value).byteLength;
}

/** Returns the longest start of `text`, in whole characters, that fits `maxBytes`. */
function truncateToBytes(text: string, maxBytes: number): string {
  let result = '';
  let used = 0;
  for (const { segment } of graphemes.segment(text)) {
    used += byteLength(segment);
    if (used > maxBytes) break;
    result += segment;
  }
  return result;
}

/**
 * Enforces a UTF-8 byte limit on a text field. Server limits count bytes, and
 * `maxlength` counts UTF-16 code units, so `maxlength` alone accepts text that
 * the server rejects.
 *
 * An edit that would exceed the limit keeps as much of the inserted text as
 * fits, in whole characters. The rest of the field stays unchanged.
 *
 * - `accept` applies the limit after an edit and returns the value to keep.
 * - `track` records a value set from outside the field, such as a loaded draft.
 */
export function createByteLimit(getMaxBytes: () => number) {
  let accepted = '';

  return {
    track(value: string) {
      if (byteLength(value) <= getMaxBytes()) accepted = value;
    },
    accept(field: TextField): string {
      const next = field.value;
      if (byteLength(next) <= getMaxBytes()) {
        accepted = next;
        return accepted;
      }

      // Find the inserted text: everything between the parts that the edit
      // kept from the start and the end of the last accepted value.
      let prefix = 0;
      while (prefix < accepted.length && accepted[prefix] === next[prefix]) prefix++;
      let suffix = 0;
      while (
        suffix < accepted.length - prefix &&
        suffix < next.length - prefix &&
        accepted[accepted.length - 1 - suffix] === next[next.length - 1 - suffix]
      ) {
        suffix++;
      }
      const before = next.slice(0, prefix);
      const after = next.slice(next.length - suffix);
      const inserted = next.slice(prefix, next.length - suffix);
      const kept = truncateToBytes(
        inserted,
        Math.max(0, getMaxBytes() - byteLength(before + after))
      );
      const limited = before + kept + after;

      accepted = byteLength(limited) <= getMaxBytes() ? limited : accepted;
      field.value = accepted;
      if (accepted === limited) {
        const caret = before.length + kept.length;
        field.setSelectionRange(caret, caret);
      }
      return accepted;
    }
  };
}
