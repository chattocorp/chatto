import { m } from '$lib/i18n/messages';

const textEncoder = new TextEncoder();

type TextField = HTMLInputElement | HTMLTextAreaElement;

/** Share of the limit at which the field starts to warn. */
const NEAR_LIMIT = 0.9;

/**
 * Returns the field description, followed by a length notice when `value`
 * comes near `maxBytes`. The notice does not name the byte count: people
 * think in characters, and a character can use several bytes.
 */
export function byteLimitDescription(
  description: string | undefined,
  maxBytes: number | undefined,
  value: string
) {
  if (maxBytes === undefined) return description;
  const used = textEncoder.encode(value).byteLength;
  const notice =
    used >= maxBytes
      ? m('ui.form.length_limit_reached')
      : used >= maxBytes * NEAR_LIMIT
        ? m('ui.form.length_limit_near')
        : undefined;
  return [description, notice].filter(Boolean).join(' ') || undefined;
}

/**
 * Enforces a UTF-8 byte limit on a text field. Server limits count bytes, and
 * `maxlength` counts UTF-16 code units, so `maxlength` alone accepts text that
 * the server rejects.
 *
 * - `beforeinput` prevents an insertion that would exceed the limit.
 * - `accept` restores the last accepted value after an edit that `beforeinput`
 *   could not prevent, and returns the value to keep.
 * - `track` records a value set from outside the field, such as a loaded draft.
 */
export function createByteLimit(getMaxBytes: () => number) {
  let accepted = '';
  const fits = (value: string) => textEncoder.encode(value).byteLength <= getMaxBytes();

  return {
    track(value: string) {
      if (fits(value)) accepted = value;
    },
    beforeinput(event: InputEvent) {
      if (!event.inputType.startsWith('insert')) return;
      const field = event.currentTarget as TextField;
      const insertedText =
        event.data ??
        event.dataTransfer?.getData('text/plain') ??
        (event.inputType === 'insertLineBreak' || event.inputType === 'insertParagraph'
          ? '\n'
          : null);
      if (insertedText == null) return;

      const start = field.selectionStart ?? field.value.length;
      const end = field.selectionEnd ?? start;
      const nextValue = field.value.slice(0, start) + insertedText + field.value.slice(end);
      if (!fits(nextValue)) event.preventDefault();
    },
    accept(field: TextField): string {
      if (fits(field.value)) {
        accepted = field.value;
      } else {
        field.value = accepted;
      }
      return accepted;
    }
  };
}
