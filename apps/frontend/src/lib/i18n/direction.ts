export type TextDirection = 'ltr' | 'rtl';

/** Return the direction currently applied to the document shell. */
export function getDocumentDirection(): TextDirection {
  return globalThis.document?.documentElement.dir === 'rtl' ? 'rtl' : 'ltr';
}

/** Convert a physical horizontal delta into a logical inline-end delta. */
export function toInlineEndDelta(
  delta: number,
  direction: TextDirection = getDocumentDirection()
): number {
  return direction === 'rtl' ? -delta : delta;
}

/** Return a transition offset that enters from the inline end edge. */
export function fromInlineEndOffset(
  offset: number,
  direction: TextDirection = getDocumentDirection()
): number {
  return direction === 'rtl' ? -offset : offset;
}
