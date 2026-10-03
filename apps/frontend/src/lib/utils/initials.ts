/** Generate local avatar labels and stable account colours independently of name validation. */
const graphemes = new Intl.Segmenter(undefined, { granularity: 'grapheme' });

/** A neutral user icon is used when no text or emoji label is available. */
export type AvatarLabel = { kind: 'text' | 'emoji'; text: string } | { kind: 'icon' };

/** Ignore blank letter fillers without removing marks or joiners from a grapheme. */
function hasVisibleInitial(segment: string): boolean {
  return [...segment].some(
    (char) => /[\p{L}\p{N}]/u.test(char) && !/\p{Default_Ignorable_Code_Point}/u.test(char)
  );
}

/**
 * Choose up to two letter/number initials, then the first complete emoji, then a
 * login initial. Grapheme segmentation keeps combining scripts and joined emoji
 * intact. Words with no letters or numbers do not contribute an initial.
 */
export function getAvatarLabel(
  displayName: string | null | undefined,
  login: string | null | undefined
): AvatarLabel {
  const name = displayName?.trim() ?? '';
  const initials: string[] = [];
  for (const word of name.split(/\s+/)) {
    for (const { segment } of graphemes.segment(word)) {
      if (hasVisibleInitial(segment)) {
        initials.push(segment.toUpperCase());
        break;
      }
    }
    if (initials.length === 2) break;
  }
  if (initials.length) return { kind: 'text', text: initials.join('') };

  for (const { segment } of graphemes.segment(name)) {
    if (/[\p{Extended_Pictographic}\p{Regional_Indicator}\u20e3]/u.test(segment)) {
      return { kind: 'emoji', text: segment };
    }
  }
  for (const { segment } of graphemes.segment(login?.trim() ?? '')) {
    if (hasVisibleInitial(segment)) {
      return { kind: 'text', text: segment.toUpperCase() };
    }
  }
  return { kind: 'icon' };
}

/** Ordered semantic palette; keep its order stable to preserve account colours. */
const AVATAR_COLOURS = [
  'blue',
  'cyan',
  'teal',
  'green',
  'amber',
  'orange',
  'pink',
  'violet'
] as const;

/** Hash an immutable account ID with UTF-8 FNV-1a to select one of eight colours. */
export function getAvatarColour(userId: string): (typeof AVATAR_COLOURS)[number] {
  let hash = 0x811c9dc5;
  for (const byte of new TextEncoder().encode(userId)) {
    hash = Math.imul(hash ^ byte, 0x01000193) >>> 0;
  }
  return AVATAR_COLOURS[hash % AVATAR_COLOURS.length];
}
